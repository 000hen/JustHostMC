package grpcsvc

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type archivedAsset struct {
	archivePath string
	entryPath   string
}

type itemAsset struct {
	ModelJSON string
	Textures  map[string][]byte
}

type modelDocument struct {
	Parent   string                    `json:"parent"`
	Textures map[string]string         `json:"textures"`
	Elements []modelElement            `json:"elements"`
	Display  map[string]modelTransform `json:"display"`
}

type modelElement struct {
	From     [3]float64           `json:"from"`
	To       [3]float64           `json:"to"`
	Rotation *modelRotation       `json:"rotation,omitempty"`
	Faces    map[string]modelFace `json:"faces"`
}

type modelRotation struct {
	Origin [3]float64 `json:"origin"`
	Axis   string     `json:"axis"`
	Angle  float64    `json:"angle"`
}

type modelFace struct {
	UV       [4]float64 `json:"uv"`
	Texture  string     `json:"texture"`
	Rotation int        `json:"rotation,omitempty"`
}

type modelTransform struct {
	Rotation    [3]float64 `json:"rotation"`
	Translation [3]float64 `json:"translation"`
	Scale       [3]float64 `json:"scale"`
}

type resolvedModel struct {
	Textures map[string]string `json:"textures"`
	Elements []modelElement    `json:"elements,omitempty"`
	GUI      modelTransform    `json:"gui"`
	Special  string            `json:"special,omitempty"`
}

// itemAssetResolver reads models and texture bytes from local client assets,
// resource packs, and mod/plugin JARs. Rendering stays in the WinUI process.
type itemAssetResolver struct {
	assets map[string]archivedAsset
	cache  map[string]itemAsset
}

func newItemAssetResolver(serverDir, minecraftVersion string, clientArchives ...string) *itemAssetResolver {
	r := &itemAssetResolver{
		assets: make(map[string]archivedAsset),
		cache:  make(map[string]itemAsset),
	}
	if client := localMinecraftClient(minecraftVersion); client != "" {
		r.indexArchive(client)
	}
	for _, client := range clientArchives {
		if strings.TrimSpace(client) != "" {
			r.indexArchive(client)
		}
	}
	// Later sources override earlier ones, matching resource-pack precedence.
	for _, dirName := range []string{"mods", "plugins", "resourcepacks"} {
		for _, path := range localAssetArchives(filepath.Join(serverDir, dirName)) {
			r.indexArchive(path)
		}
	}
	for _, name := range []string{"resources.zip", "resourcepack.zip"} {
		path := filepath.Join(serverDir, name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			r.indexArchive(path)
		}
	}
	return r
}

func (r *itemAssetResolver) Resolve(itemID string) itemAsset {
	itemID = strings.ToLower(strings.TrimSpace(itemID))
	if cached, ok := r.cache[itemID]; ok {
		return cached
	}
	namespace, itemPath, ok := splitAssetID(itemID, "minecraft")
	if !ok {
		return itemAsset{}
	}

	modelRef := ""
	var model resolvedModel
	found := false
	if definition := r.readJSON("assets/" + namespace + "/items/" + itemPath + ".json"); definition != nil {
		if base, kind, texture, special := specialItemModel(definition); special {
			if model, found = r.loadModel(base, 0); found && kind == "minecraft:chest" {
				if textureNamespace, texturePath, ok := splitAssetID(texture, "minecraft"); ok {
					model.Special = "chest"
					model.Textures["special"] = textureNamespace + ":entity/chest/" + texturePath
				}
			}
		}
		modelRef = findStringField(definition, "model")
	}
	if !found {
		if modelRef == "" {
			modelRef = namespace + ":item/" + itemPath
		}
		model, found = r.loadModel(modelRef, 0)
	}
	if !found {
		model = resolvedModel{Textures: make(map[string]string)}
		for _, direct := range []string{namespace + ":item/" + itemPath, namespace + ":block/" + itemPath} {
			if len(r.readTexture(direct, namespace)) > 0 {
				model.Textures["layer0"] = direct
				found = true
				break
			}
		}
	}
	if !found {
		return itemAsset{}
	}

	asset := itemAsset{Textures: make(map[string][]byte)}
	if encoded, err := json.Marshal(model); err == nil {
		asset.ModelJSON = string(encoded)
	}
	for _, ref := range modelTextureRefs(model) {
		if data := r.readTexture(ref, "minecraft"); len(data) > 0 {
			asset.Textures[ref] = data
		}
	}
	r.cache[itemID] = asset
	return asset
}

func specialItemModel(definition any) (base, kind, texture string, ok bool) {
	root, ok := definition.(map[string]any)
	if !ok {
		return "", "", "", false
	}
	model, ok := root["model"].(map[string]any)
	if !ok || model["type"] != "minecraft:special" {
		return "", "", "", false
	}
	base, _ = model["base"].(string)
	special, _ := model["model"].(map[string]any)
	kind, _ = special["type"].(string)
	texture, _ = special["texture"].(string)
	return base, kind, texture, base != "" && kind != "" && texture != ""
}

func (r *itemAssetResolver) loadModel(modelRef string, depth int) (resolvedModel, bool) {
	if depth > 16 {
		return resolvedModel{}, false
	}
	namespace, modelPath, ok := splitAssetID(modelRef, "minecraft")
	if !ok {
		return resolvedModel{}, false
	}
	data := r.readAsset("assets/" + namespace + "/models/" + modelPath + ".json")
	if len(data) == 0 {
		return resolvedModel{}, false
	}
	var document modelDocument
	if json.Unmarshal(data, &document) != nil {
		return resolvedModel{}, false
	}

	model := resolvedModel{Textures: make(map[string]string)}
	if document.Parent != "" {
		if parent, found := r.loadModel(document.Parent, depth+1); found {
			model = parent
		}
	}
	if model.Textures == nil {
		model.Textures = make(map[string]string)
	}
	for key, value := range document.Textures {
		model.Textures[key] = value
	}
	if document.Elements != nil {
		model.Elements = document.Elements
	}
	if gui, found := document.Display["gui"]; found {
		model.GUI = gui
	}
	return model, true
}

func modelTextureRefs(model resolvedModel) []string {
	set := make(map[string]struct{})
	for _, ref := range model.Textures {
		if resolved := resolveTextureVariable(ref, model.Textures); resolved != "" {
			set[resolved] = struct{}{}
		}
	}
	for _, element := range model.Elements {
		for _, face := range element.Faces {
			if resolved := resolveTextureVariable(face.Texture, model.Textures); resolved != "" {
				set[resolved] = struct{}{}
			}
		}
	}
	refs := make([]string, 0, len(set))
	for ref := range set {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs
}

func resolveTextureVariable(ref string, textures map[string]string) string {
	for index := 0; strings.HasPrefix(ref, "#") && index < 16; index++ {
		ref = textures[strings.TrimPrefix(ref, "#")]
	}
	return ref
}

func localMinecraftClient(version string) string {
	versionsDirs := make([]string, 0, 2)
	if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
		versionsDirs = append(versionsDirs, filepath.Join(appData, ".minecraft", "versions"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		fallback := filepath.Join(home, "AppData", "Roaming", ".minecraft", "versions")
		if len(versionsDirs) == 0 || !strings.EqualFold(versionsDirs[0], fallback) {
			versionsDirs = append(versionsDirs, fallback)
		}
	}

	version = strings.TrimSpace(version)
	for _, versionsDir := range versionsDirs {
		if version == "" {
			continue
		}
		exact := filepath.Join(versionsDir, version, version+".jar")
		if info, err := os.Stat(exact); err == nil && info.Mode().IsRegular() {
			return exact
		}
	}

	// Never substitute another Minecraft version. A visually plausible older
	// model is worse than a missing icon and cannot represent newly-added blocks.
	return ""
}

func localAssetArchives(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".jar" || ext == ".zip" {
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(paths)
	return paths
}

func (r *itemAssetResolver) indexArchive(path string) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return
	}
	defer zr.Close()
	for _, file := range zr.File {
		entry := normalizeAssetPath(file.Name)
		if strings.HasPrefix(entry, "assets/") && (strings.HasSuffix(entry, ".json") || strings.HasSuffix(entry, ".png")) {
			r.assets[entry] = archivedAsset{archivePath: path, entryPath: file.Name}
		}
	}
}

func (r *itemAssetResolver) readTexture(ref, defaultNamespace string) []byte {
	namespace, texturePath, ok := splitAssetID(ref, defaultNamespace)
	if !ok {
		return nil
	}
	return r.readAsset("assets/" + namespace + "/textures/" + texturePath + ".png")
}

func (r *itemAssetResolver) readJSON(path string) any {
	data := r.readAsset(path)
	if len(data) == 0 {
		return nil
	}
	var value any
	if json.Unmarshal(data, &value) != nil {
		return nil
	}
	return value
}

func (r *itemAssetResolver) readAsset(path string) []byte {
	asset, ok := r.assets[normalizeAssetPath(path)]
	if !ok {
		return nil
	}
	zr, err := zip.OpenReader(asset.archivePath)
	if err != nil {
		return nil
	}
	defer zr.Close()
	for _, file := range zr.File {
		if file.Name != asset.entryPath {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil
		}
		defer rc.Close()
		data, err := io.ReadAll(io.LimitReader(rc, 4<<20))
		if err == nil {
			return data
		}
	}
	return nil
}

func splitAssetID(value, defaultNamespace string) (string, string, bool) {
	value = strings.TrimSpace(strings.TrimPrefix(value, "/"))
	namespace, path := defaultNamespace, value
	if before, after, found := strings.Cut(value, ":"); found {
		namespace, path = before, after
	}
	if namespace == "" || path == "" || strings.Contains(namespace, "..") || strings.Contains(path, "..") {
		return "", "", false
	}
	return namespace, strings.TrimPrefix(path, "/"), true
}

func normalizeAssetPath(path string) string {
	return strings.ToLower(strings.TrimPrefix(strings.ReplaceAll(path, `\`, "/"), "/"))
}

func findStringField(value any, field string) string {
	switch typed := value.(type) {
	case map[string]any:
		if text, ok := typed[field].(string); ok {
			return text
		}
		for _, child := range typed {
			if found := findStringField(child, field); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := findStringField(child, field); found != "" {
				return found
			}
		}
	}
	return ""
}
