package grpcsvc

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
	"github.com/000hen/justhostmc/engine/internal/appdata"
	"github.com/000hen/justhostmc/engine/internal/store"
	"github.com/Tnze/go-mc/nbt"
	"github.com/Tnze/go-mc/nbt/dynbt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ConfigService edits Minecraft's file-backed configuration. server.properties
// is a Java-style key/value file; gamerules live inside world/level.dat as NBT.
type ConfigService struct {
	mcmanagerv1.UnimplementedConfigServiceServer
	store store.Store
	paths appdata.Paths
}

func NewConfigService(st store.Store, paths appdata.Paths) *ConfigService {
	return &ConfigService{store: st, paths: paths}
}

func (s *ConfigService) GetServerProperties(_ context.Context, req *mcmanagerv1.ServerId) (*mcmanagerv1.ServerProperties, error) {
	rec, ok := s.store.Get(req.Id)
	if !ok {
		return nil, status.Error(codes.NotFound, "server not found")
	}
	props, err := readPropertiesFile(filepath.Join(s.paths.ServerDir(req.Id), "server.properties"))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "read server.properties: %v", err)
	}
	return &mcmanagerv1.ServerProperties{
		ServerId: req.Id,
		Entries:  propertyEntries(rec.McVersion, props),
	}, nil
}

func (s *ConfigService) UpdateServerProperties(ctx context.Context, req *mcmanagerv1.UpdateServerPropertiesRequest) (*mcmanagerv1.ServerProperties, error) {
	rec, ok := s.store.Get(req.ServerId)
	if !ok {
		return nil, status.Error(codes.NotFound, "server not found")
	}
	if !isEditableStopped(rec.Status) {
		return nil, errorStatus(codes.FailedPrecondition, mcmanagerv1.ErrorCode_SERVER_RUNNING,
			"stop the server before changing server.properties", nil)
	}
	if len(req.Entries) == 0 {
		return s.GetServerProperties(ctx, &mcmanagerv1.ServerId{Id: req.ServerId})
	}

	schemas := propertySchemaFor(rec.McVersion)
	updates := make(map[string]string, len(req.Entries))
	for _, entry := range req.Entries {
		key := strings.TrimSpace(entry.GetKey())
		if key == "" {
			return nil, status.Error(codes.InvalidArgument, "property key is required")
		}
		schema := schemas[key]
		value, err := normalizeConfigValue(schema, entry.GetValue())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "%s: %v", key, err)
		}
		if isPortProperty(key) {
			port, _ := strconv.Atoi(value)
			if port <= 0 || port > 65535 {
				return nil, status.Error(codes.InvalidArgument, key+" must be between 1 and 65535")
			}
			if key == "server-port" && port != rec.Port && !isPortFree(port) {
				return nil, errorStatus(codes.AlreadyExists, mcmanagerv1.ErrorCode_PORT_IN_USE,
					fmt.Sprintf("port %d is already in use", port), map[string]string{"port": value})
			}
		}
		updates[key] = value
	}
	if serverPort, ok := updates["server-port"]; ok {
		if _, hasQueryPort := updates["query.port"]; !hasQueryPort {
			updates["query.port"] = serverPort
		}
	}

	path := filepath.Join(s.paths.ServerDir(req.ServerId), "server.properties")
	if err := writePropertiesUpdates(path, updates); err != nil {
		return nil, status.Errorf(codes.Internal, "write server.properties: %v", err)
	}
	if serverPort, ok := updates["server-port"]; ok {
		if port, err := strconv.Atoi(serverPort); err == nil {
			rec.Port = port
			_ = s.store.Put(rec)
		}
	}
	return s.GetServerProperties(ctx, &mcmanagerv1.ServerId{Id: req.ServerId})
}

func (s *ConfigService) GetGameRules(_ context.Context, req *mcmanagerv1.ServerId) (*mcmanagerv1.GameRules, error) {
	rec, ok := s.store.Get(req.Id)
	if !ok {
		return nil, status.Error(codes.NotFound, "server not found")
	}
	path := filepath.Join(s.paths.ServerDir(req.Id), "world", "level.dat")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return &mcmanagerv1.GameRules{
				ServerId:    req.Id,
				WorldExists: false,
				Message:     "Start the server once to create a world before editing gamerules.",
				Entries:     gameRuleEntries(rec.McVersion, nil),
			}, nil
		}
		return nil, status.Errorf(codes.Internal, "stat level.dat: %v", err)
	}

	var root dynbt.Value
	if _, err := readNBTFile(path, &root); err != nil {
		return nil, status.Errorf(codes.Internal, "read level.dat: %v", err)
	}
	values := make(map[string]string)
	if rules := root.Get("Data", "GameRules"); rules != nil {
		for _, schema := range gameRuleCatalog {
			if v := rules.Get(schema.Key); v != nil {
				values[schema.Key] = v.String()
			}
		}
	}

	return &mcmanagerv1.GameRules{
		ServerId:    req.Id,
		WorldExists: true,
		Entries:     gameRuleEntries(rec.McVersion, values),
	}, nil
}

func (s *ConfigService) UpdateGameRules(ctx context.Context, req *mcmanagerv1.UpdateGameRulesRequest) (*mcmanagerv1.GameRules, error) {
	rec, ok := s.store.Get(req.ServerId)
	if !ok {
		return nil, status.Error(codes.NotFound, "server not found")
	}
	if !isEditableStopped(rec.Status) {
		return nil, errorStatus(codes.FailedPrecondition, mcmanagerv1.ErrorCode_SERVER_RUNNING,
			"stop the server before changing gamerules", nil)
	}
	path := filepath.Join(s.paths.ServerDir(req.ServerId), "world", "level.dat")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, status.Error(codes.FailedPrecondition, "world has not been created yet")
		}
		return nil, status.Errorf(codes.Internal, "stat level.dat: %v", err)
	}

	schemas := gameRuleSchemaFor(rec.McVersion)
	updates := make(map[string]string, len(req.Entries))
	for _, entry := range req.Entries {
		key := strings.TrimSpace(entry.GetKey())
		schema, ok := schemas[key]
		if !ok {
			return nil, status.Errorf(codes.InvalidArgument, "gamerule %s is not supported by Minecraft %s", key, rec.McVersion)
		}
		value, err := normalizeConfigValue(schema, entry.GetValue())
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "%s: %v", key, err)
		}
		updates[key] = value
	}
	if len(updates) == 0 {
		return s.GetGameRules(ctx, &mcmanagerv1.ServerId{Id: req.ServerId})
	}

	var root dynbt.Value
	rootName, err := readNBTFile(path, &root)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "read level.dat: %v", err)
	}
	data := root.Get("Data")
	if data == nil {
		data = dynbt.NewCompound()
		root.Set("Data", data)
	}
	rules := data.Get("GameRules")
	if rules == nil {
		rules = dynbt.NewCompound()
		data.Set("GameRules", rules)
	}
	for key, value := range updates {
		rules.Set(key, dynbt.NewString(value))
	}
	if err := writeNBTFile(path, rootName, &root); err != nil {
		return nil, status.Errorf(codes.Internal, "write level.dat: %v", err)
	}
	return s.GetGameRules(ctx, &mcmanagerv1.ServerId{Id: req.ServerId})
}

type configSchema struct {
	Key     string
	Type    mcmanagerv1.ConfigValueType
	Default string
	Choices []string
	Since   string
	Removed string
	Desc    string
}

func propertySchemaFor(version string) map[string]configSchema {
	out := make(map[string]configSchema)
	for _, schema := range propertyCatalog {
		if schema.supports(version) {
			out[schema.Key] = schema
		}
	}
	return out
}

func gameRuleSchemaFor(version string) map[string]configSchema {
	out := make(map[string]configSchema)
	for _, schema := range gameRuleCatalog {
		if schema.supports(version) {
			out[schema.Key] = schema
		}
	}
	return out
}

func (s configSchema) supports(version string) bool {
	if s.Since != "" && compareMCVersion(version, s.Since) < 0 {
		return false
	}
	if s.Removed != "" && compareMCVersion(version, s.Removed) >= 0 {
		return false
	}
	return true
}

func propertyEntries(version string, values map[string]string) []*mcmanagerv1.ConfigEntry {
	entries := make([]*mcmanagerv1.ConfigEntry, 0, len(propertyCatalog)+len(values))
	seen := make(map[string]bool)
	for _, schema := range propertyCatalog {
		if !schema.supports(version) {
			continue
		}
		value := schema.Default
		if actual, ok := values[schema.Key]; ok {
			value = actual
		}
		entries = append(entries, schemaEntry(schema, value, true))
		seen[schema.Key] = true
	}
	var unknown []string
	for key := range values {
		if !seen[key] {
			unknown = append(unknown, key)
		}
	}
	sort.Strings(unknown)
	for _, key := range unknown {
		entries = append(entries, &mcmanagerv1.ConfigEntry{
			Key:       key,
			Value:     values[key],
			Type:      mcmanagerv1.ConfigValueType_CONFIG_STRING,
			Supported: true,
		})
	}
	return entries
}

func gameRuleEntries(version string, values map[string]string) []*mcmanagerv1.ConfigEntry {
	entries := make([]*mcmanagerv1.ConfigEntry, 0, len(gameRuleCatalog))
	for _, schema := range gameRuleCatalog {
		if !schema.supports(version) {
			continue
		}
		value := schema.Default
		if actual, ok := values[schema.Key]; ok {
			value = actual
		}
		entries = append(entries, schemaEntry(schema, value, true))
	}
	return entries
}

func schemaEntry(schema configSchema, value string, supported bool) *mcmanagerv1.ConfigEntry {
	return &mcmanagerv1.ConfigEntry{
		Key:          schema.Key,
		Value:        value,
		Type:         schema.Type,
		Choices:      append([]string(nil), schema.Choices...),
		Description:  schema.Desc,
		Supported:    supported,
		SinceVersion: schema.Since,
	}
}

func normalizeConfigValue(schema configSchema, value string) (string, error) {
	value = strings.TrimSpace(value)
	switch schema.Type {
	case mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN:
		switch strings.ToLower(value) {
		case "true", "1", "yes", "on":
			return "true", nil
		case "false", "0", "no", "off":
			return "false", nil
		default:
			return "", fmt.Errorf("must be true or false")
		}
	case mcmanagerv1.ConfigValueType_CONFIG_INTEGER:
		if _, err := strconv.Atoi(value); err != nil {
			return "", fmt.Errorf("must be an integer")
		}
		return value, nil
	case mcmanagerv1.ConfigValueType_CONFIG_ENUM:
		lower := strings.ToLower(value)
		for _, choice := range schema.Choices {
			if lower == strings.ToLower(choice) {
				return choice, nil
			}
		}
		return "", fmt.Errorf("must be one of %s", strings.Join(schema.Choices, ", "))
	default:
		return value, nil
	}
}

func isPortProperty(key string) bool {
	return key == "server-port" || key == "query.port" || key == "rcon.port"
}

type propertyLine struct {
	raw   string
	key   string
	value string
}

func readPropertiesFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	lines := parsePropertyLines(string(data))
	out := make(map[string]string)
	for _, line := range lines {
		if line.key != "" {
			out[line.key] = line.value
		}
	}
	return out, nil
}

func writePropertiesUpdates(path string, updates map[string]string) error {
	var lines []propertyLine
	data, err := os.ReadFile(path)
	if err == nil {
		lines = parsePropertyLines(string(data))
	} else if !os.IsNotExist(err) {
		return err
	}
	seen := make(map[string]bool)
	for i := range lines {
		if value, ok := updates[lines[i].key]; ok {
			lines[i].raw = lines[i].key + "=" + value
			lines[i].value = value
			seen[lines[i].key] = true
		}
	}
	var missing []string
	for key := range updates {
		if !seen[key] {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	for _, key := range missing {
		lines = append(lines, propertyLine{raw: key + "=" + updates[key], key: key, value: updates[key]})
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(line.raw)
		b.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func parsePropertyLines(text string) []propertyLine {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	rawLines := strings.Split(text, "\n")
	lines := make([]propertyLine, 0, len(rawLines))
	for _, raw := range rawLines {
		if raw == "" {
			continue
		}
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "!") {
			lines = append(lines, propertyLine{raw: raw})
			continue
		}
		idx := strings.IndexAny(raw, "=:")
		if idx < 0 {
			lines = append(lines, propertyLine{raw: raw})
			continue
		}
		key := strings.TrimSpace(raw[:idx])
		value := strings.TrimSpace(raw[idx+1:])
		lines = append(lines, propertyLine{raw: raw, key: key, value: value})
	}
	return lines
}

func readNBTFile(path string, v any) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var r io.Reader = f
	gz, err := gzip.NewReader(f)
	if err == nil {
		defer gz.Close()
		r = gz
	} else {
		if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
			return "", seekErr
		}
	}
	return nbt.NewDecoder(r).Decode(v)
}

func writeNBTFile(path, rootName string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".level-*.dat")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()

	gz := gzip.NewWriter(tmp)
	if err := nbt.NewEncoder(gz).Encode(v, rootName); err != nil {
		_ = gz.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	committed = true
	return nil
}

var mcVersionPart = regexp.MustCompile(`\d+`)

func compareMCVersion(a, b string) int {
	ap := parseMCVersion(a)
	bp := parseMCVersion(b)
	for i := 0; i < len(ap) || i < len(bp); i++ {
		av, bv := 0, 0
		if i < len(ap) {
			av = ap[i]
		}
		if i < len(bp) {
			bv = bp[i]
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}

func parseMCVersion(v string) []int {
	matches := mcVersionPart.FindAllString(v, 4)
	parts := make([]int, 0, len(matches))
	for _, m := range matches {
		n, _ := strconv.Atoi(m)
		parts = append(parts, n)
	}
	if len(parts) == 0 {
		return []int{0}
	}
	return parts
}

var boolChoices = []string{"true", "false"}

var propertyCatalog = []configSchema{
	{Key: "accepts-transfers", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "false", Since: "1.20.5"},
	{Key: "allow-flight", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "false"},
	{Key: "allow-nether", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "broadcast-console-to-ops", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "broadcast-rcon-to-ops", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "bug-report-link", Type: mcmanagerv1.ConfigValueType_CONFIG_STRING, Default: "", Since: "1.20"},
	{Key: "difficulty", Type: mcmanagerv1.ConfigValueType_CONFIG_ENUM, Default: "easy", Choices: []string{"peaceful", "easy", "normal", "hard"}},
	{Key: "enable-command-block", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "false"},
	{Key: "enable-jmx-monitoring", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "false"},
	{Key: "enable-query", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "false"},
	{Key: "enable-rcon", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "false"},
	{Key: "enable-status", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "enforce-secure-profile", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true", Since: "1.19"},
	{Key: "enforce-whitelist", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "false"},
	{Key: "entity-broadcast-range-percentage", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "100"},
	{Key: "force-gamemode", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "false"},
	{Key: "function-permission-level", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "2"},
	{Key: "gamemode", Type: mcmanagerv1.ConfigValueType_CONFIG_ENUM, Default: "survival", Choices: []string{"survival", "creative", "adventure", "spectator"}},
	{Key: "generate-structures", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "generator-settings", Type: mcmanagerv1.ConfigValueType_CONFIG_STRING, Default: "{}"},
	{Key: "hardcore", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "false"},
	{Key: "hide-online-players", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "false"},
	{Key: "initial-disabled-packs", Type: mcmanagerv1.ConfigValueType_CONFIG_STRING, Default: "", Since: "1.20"},
	{Key: "initial-enabled-packs", Type: mcmanagerv1.ConfigValueType_CONFIG_STRING, Default: "vanilla", Since: "1.20"},
	{Key: "level-name", Type: mcmanagerv1.ConfigValueType_CONFIG_STRING, Default: "world"},
	{Key: "level-seed", Type: mcmanagerv1.ConfigValueType_CONFIG_STRING, Default: ""},
	{Key: "level-type", Type: mcmanagerv1.ConfigValueType_CONFIG_STRING, Default: "minecraft:normal"},
	{Key: "log-ips", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true", Since: "1.19.4"},
	{Key: "max-chained-neighbor-updates", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "1000000", Since: "1.19"},
	{Key: "max-players", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "20"},
	{Key: "max-tick-time", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "60000"},
	{Key: "max-world-size", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "29999984"},
	{Key: "motd", Type: mcmanagerv1.ConfigValueType_CONFIG_STRING, Default: "A Minecraft Server"},
	{Key: "network-compression-threshold", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "256"},
	{Key: "online-mode", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "op-permission-level", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "4"},
	{Key: "pause-when-empty-seconds", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "-1", Since: "1.21.4"},
	{Key: "player-idle-timeout", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "0"},
	{Key: "prevent-proxy-connections", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "false"},
	{Key: "pvp", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "query.port", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "25565"},
	{Key: "rate-limit", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "0"},
	{Key: "rcon.password", Type: mcmanagerv1.ConfigValueType_CONFIG_STRING, Default: ""},
	{Key: "rcon.port", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "25575"},
	{Key: "region-file-compression", Type: mcmanagerv1.ConfigValueType_CONFIG_ENUM, Default: "deflate", Choices: []string{"deflate", "lz4", "none"}, Since: "1.20.5"},
	{Key: "require-resource-pack", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "false"},
	{Key: "resource-pack", Type: mcmanagerv1.ConfigValueType_CONFIG_STRING, Default: ""},
	{Key: "resource-pack-id", Type: mcmanagerv1.ConfigValueType_CONFIG_STRING, Default: "", Since: "1.20"},
	{Key: "resource-pack-prompt", Type: mcmanagerv1.ConfigValueType_CONFIG_STRING, Default: ""},
	{Key: "resource-pack-sha1", Type: mcmanagerv1.ConfigValueType_CONFIG_STRING, Default: ""},
	{Key: "server-ip", Type: mcmanagerv1.ConfigValueType_CONFIG_STRING, Default: ""},
	{Key: "server-port", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "25565"},
	{Key: "simulation-distance", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "10", Since: "1.18"},
	{Key: "spawn-animals", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "spawn-monsters", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "spawn-npcs", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "spawn-protection", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "16"},
	{Key: "sync-chunk-writes", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "text-filtering-config", Type: mcmanagerv1.ConfigValueType_CONFIG_STRING, Default: ""},
	{Key: "use-native-transport", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "view-distance", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "10"},
	{Key: "white-list", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "false"},
}

var gameRuleCatalog = []configSchema{
	{Key: "announceAdvancements", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "blockExplosionDropDecay", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true", Since: "1.19.3"},
	{Key: "commandBlockOutput", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "commandModificationBlockLimit", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "32768", Since: "1.19.4"},
	{Key: "disableElytraMovementCheck", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "false"},
	{Key: "disableRaids", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "false", Since: "1.14"},
	{Key: "doDaylightCycle", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "doEntityDrops", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "doFireTick", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "doImmediateRespawn", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "false", Since: "1.15"},
	{Key: "doInsomnia", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true", Since: "1.15"},
	{Key: "doLimitedCrafting", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "false", Since: "1.12"},
	{Key: "doMobLoot", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "doMobSpawning", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "doPatrolSpawning", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true", Since: "1.15"},
	{Key: "doTileDrops", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "doTraderSpawning", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true", Since: "1.15"},
	{Key: "doVinesSpread", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true", Since: "1.18"},
	{Key: "doWardenSpawning", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true", Since: "1.19"},
	{Key: "doWeatherCycle", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "drowningDamage", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true", Since: "1.15"},
	{Key: "enderPearlsVanishOnDeath", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true", Since: "1.21.2"},
	{Key: "fallDamage", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true", Since: "1.15"},
	{Key: "fireDamage", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true", Since: "1.15"},
	{Key: "forgiveDeadPlayers", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true", Since: "1.16"},
	{Key: "freezeDamage", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true", Since: "1.17"},
	{Key: "globalSoundEvents", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true", Since: "1.19"},
	{Key: "keepInventory", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "false"},
	{Key: "lavaSourceConversion", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "false", Since: "1.19.3"},
	{Key: "logAdminCommands", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "maxCommandChainLength", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "65536", Since: "1.12"},
	{Key: "maxCommandForkCount", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "65536", Since: "1.20.3"},
	{Key: "maxEntityCramming", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "24", Since: "1.11"},
	{Key: "mobExplosionDropDecay", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true", Since: "1.19.3"},
	{Key: "mobGriefing", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "naturalRegeneration", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "playersNetherPortalCreativeDelay", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "1", Since: "1.20.3"},
	{Key: "playersNetherPortalDefaultDelay", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "80", Since: "1.20.3"},
	{Key: "playersSleepingPercentage", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "100", Since: "1.17"},
	{Key: "randomTickSpeed", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "3"},
	{Key: "reducedDebugInfo", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "false", Since: "1.8"},
	{Key: "sendCommandFeedback", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "showDeathMessages", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "snowAccumulationHeight", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "1", Since: "1.19.3"},
	{Key: "spawnChunkRadius", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "2", Since: "1.20.5"},
	{Key: "spawnRadius", Type: mcmanagerv1.ConfigValueType_CONFIG_INTEGER, Default: "10"},
	{Key: "spectatorsGenerateChunks", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true"},
	{Key: "tntExplosionDropDecay", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "false", Since: "1.19.3"},
	{Key: "universalAnger", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "false", Since: "1.16"},
	{Key: "waterSourceConversion", Type: mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN, Default: "true", Since: "1.19.3"},
}

func init() {
	for i := range propertyCatalog {
		if propertyCatalog[i].Type == mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN {
			propertyCatalog[i].Choices = boolChoices
		}
	}
	for i := range gameRuleCatalog {
		if gameRuleCatalog[i].Type == mcmanagerv1.ConfigValueType_CONFIG_BOOLEAN {
			gameRuleCatalog[i].Choices = boolChoices
		}
	}
}
