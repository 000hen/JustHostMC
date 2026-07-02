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

// itemAsset is deliberately opaque to the engine. The WinUI client owns all
// item-definition, model-inheritance, texture, tint, and rendering semantics.
type itemAsset struct {
	Files map[string][]byte
}

// itemAssetResolver only locates resource-pack files and follows their declared
// resource references. It does not interpret or flatten Minecraft models.
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

	result := itemAsset{Files: make(map[string][]byte)}
	visitedModels := make(map[string]struct{})
	definitionPath := normalizeAssetPath("assets/" + namespace + "/items/" + itemPath + ".json")
	if definition := r.readAsset(definitionPath); len(definition) > 0 {
		result.Files[definitionPath] = definition
		r.collectJSONDependencies(definition, namespace, result.Files, visitedModels)
	} else {
		r.collectModel(namespace+":item/"+itemPath, namespace, result.Files, visitedModels)
	}

	if len(result.Files) == 0 {
		return itemAsset{}
	}
	r.cache[itemID] = result
	return result
}

func (r *itemAssetResolver) collectModel(
	modelRef string,
	defaultNamespace string,
	files map[string][]byte,
	visited map[string]struct{},
) {
	namespace, modelPath, ok := splitAssetID(modelRef, defaultNamespace)
	if !ok {
		return
	}
	path := normalizeAssetPath("assets/" + namespace + "/models/" + modelPath + ".json")
	if _, seen := visited[path]; seen {
		return
	}
	visited[path] = struct{}{}
	data := r.readAsset(path)
	if len(data) == 0 {
		return
	}
	files[path] = data
	r.collectJSONDependencies(data, namespace, files, visited)
}

func (r *itemAssetResolver) collectJSONDependencies(
	data []byte,
	defaultNamespace string,
	files map[string][]byte,
	visitedModels map[string]struct{},
) {
	var root any
	if json.Unmarshal(data, &root) != nil {
		return
	}
	var walk func(any, string)
	walk = func(value any, field string) {
		switch typed := value.(type) {
		case map[string]any:
			for key, child := range typed {
				walk(child, strings.ToLower(key))
			}
		case []any:
			for _, child := range typed {
				walk(child, field)
			}
		case string:
			switch field {
			case "parent", "model", "base":
				r.collectModel(typed, "minecraft", files, visitedModels)
			case "texture":
				r.collectTexture(typed, "minecraft", files)
			case "type":
				r.collectNamedTextures(typed, files)
			}
		}
	}
	walk(root, "")

	// Texture variables live in a map, so their keys are arbitrary rather than
	// literally named "texture". Collect all string values from that map.
	if object, ok := root.(map[string]any); ok {
		if textures, ok := object["textures"].(map[string]any); ok {
			for _, value := range textures {
				if ref, ok := value.(string); ok && !strings.HasPrefix(ref, "#") {
					r.collectTexture(ref, "minecraft", files)
				}
			}
		}
	}
}

// Built-in special model types declare their resource name but not every
// texture path used by Minecraft's entity renderer. Locate matching raw
// textures by the declared resource name without interpreting the model type.
func (r *itemAssetResolver) collectNamedTextures(ref string, files map[string][]byte) {
	namespace, name, ok := splitAssetID(ref, "minecraft")
	if !ok || strings.Contains(name, "/") {
		return
	}
	prefix := normalizeAssetPath("assets/" + namespace + "/textures/")
	directoryPart := "/" + strings.ToLower(name) + "/"
	filePrefix := "/" + strings.ToLower(name) + "_"
	for path := range r.assets {
		if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, ".png") {
			continue
		}
		if strings.Contains(path, directoryPart) || strings.Contains(path, filePrefix) {
			if data := r.readAsset(path); len(data) > 0 {
				files[path] = data
			}
		}
	}
}

func (r *itemAssetResolver) collectTexture(ref, defaultNamespace string, files map[string][]byte) {
	namespace, texturePath, ok := splitAssetID(ref, defaultNamespace)
	if !ok || strings.HasPrefix(texturePath, "#") {
		return
	}
	exact := normalizeAssetPath("assets/" + namespace + "/textures/" + texturePath + ".png")
	if data := r.readAsset(exact); len(data) > 0 {
		files[exact] = data
	}

	// Special model declarations use a short texture id whose directory is
	// implied by their declared model type. Send matching raw assets and let C#
	// apply that model-type rule; the engine remains format-agnostic.
	prefix := normalizeAssetPath("assets/" + namespace + "/textures/")
	suffix := "/" + strings.ToLower(texturePath) + ".png"
	for path := range r.assets {
		if strings.HasPrefix(path, prefix) && strings.HasSuffix(path, suffix) {
			if data := r.readAsset(path); len(data) > 0 {
				files[path] = data
			}
		}
	}
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
