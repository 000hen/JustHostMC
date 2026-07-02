using System.Runtime.InteropServices.WindowsRuntime;
using System.Text.Json;
using System.Text.Json.Nodes;
using McManager.Grpc;
using Microsoft.UI.Xaml.Media;
using Microsoft.UI.Xaml.Media.Imaging;
using Windows.Graphics.Imaging;
using Windows.Storage.Streams;

namespace JustHostMC.App.Services;

/// <summary>Interprets Minecraft item/model JSON and renders it from raw resource-pack assets.</summary>
internal static class MinecraftItemIconRenderer
{
    private const int Size = 64;
    private const double Epsilon = 0.000001;
    private static readonly JsonSerializerOptions JsonOptions = new() { PropertyNameCaseInsensitive = true };

    public static async Task<ImageSource?> RenderAsync(string itemId, IEnumerable<PlayerItemAsset> assetMessages)
    {
        if (!TryResourceId(itemId, "minecraft", out var itemNamespace, out var itemPath))
            return null;

        var assets = new AssetSet();
        foreach (var message in assetMessages)
            await assets.AddAsync(message);

        var parts = ResolveItemParts(itemNamespace, itemPath, assets);
        if (parts.Count == 0)
            return null;

        var quads = new List<RenderQuad>();
        foreach (var part in parts)
        {
            var model = ResolveModel(part.Model, assets, new HashSet<string>(StringComparer.OrdinalIgnoreCase));
            if (model is null)
                continue;
            if (part.Special is not null)
                AddSpecialModel(quads, model, part.Special, part.Tints, assets);
            else if (model.Generated)
                AddGeneratedModel(quads, model, part.Tints, assets);
            else
                AddBlockModel(quads, model, part.Tints, assets);
        }
        if (quads.Count == 0)
            return null;

        var pixels = Rasterize(quads);
        var bitmap = new WriteableBitmap(Size, Size);
        using (var stream = bitmap.PixelBuffer.AsStream())
            stream.Write(pixels, 0, pixels.Length);
        bitmap.Invalidate();
        return bitmap;
    }

    private static List<ItemPart> ResolveItemParts(string itemNamespace, string itemPath, AssetSet assets)
    {
        var definitionPath = $"assets/{itemNamespace}/items/{itemPath}.json";
        if (assets.Json.TryGetValue(definitionPath, out var definition))
        {
            try
            {
                var root = JsonNode.Parse(definition);
                return ResolveItemNode(root?["model"], itemNamespace);
            }
            catch (JsonException)
            {
                return [];
            }
        }
        return [new ItemPart($"{itemNamespace}:item/{itemPath}", [], null)];
    }

    private static List<ItemPart> ResolveItemNode(JsonNode? node, string defaultNamespace)
    {
        if (node is null)
            return [];
        if (node is JsonValue value && value.TryGetValue<string>(out var directModel))
            return [new ItemPart(Qualify(directModel, "minecraft"), [], null)];
        if (node is not JsonObject model)
            return [];

        var type = model["type"]?.GetValue<string>() ?? "minecraft:model";
        switch (type)
        {
            case "minecraft:model":
            {
                var modelRef = model["model"]?.GetValue<string>();
                return string.IsNullOrWhiteSpace(modelRef)
                    ? []
                    : [new ItemPart(Qualify(modelRef, "minecraft"), ReadTints(model["tints"]), null)];
            }
            case "minecraft:special":
            {
                var baseModel = model["base"]?.GetValue<string>();
                return string.IsNullOrWhiteSpace(baseModel)
                    ? []
                    : [new ItemPart(Qualify(baseModel, "minecraft"), ReadTints(model["tints"]), model["model"] as JsonObject)];
            }
            case "minecraft:composite":
            {
                var result = new List<ItemPart>();
                if (model["models"] is JsonArray children)
                    foreach (var child in children)
                        result.AddRange(ResolveItemNode(child, defaultNamespace));
                return result;
            }
            case "minecraft:condition":
                return ResolveItemNode(model["on_false"] ?? model["on_true"], defaultNamespace);
            case "minecraft:range_dispatch":
                return ResolveItemNode(model["fallback"] ?? FirstNestedModel(model), defaultNamespace);
            case "minecraft:select":
                return ResolveItemNode(SelectGuiModel(model) ?? model["fallback"] ?? FirstNestedModel(model), defaultNamespace);
            case "minecraft:empty":
                return [];
            default:
                // Unknown extension types may still wrap a normal model. Following
                // their declared fallback is safer than item-name special cases.
                return ResolveItemNode(model["fallback"] ?? model["model"], defaultNamespace);
        }
    }

    private static JsonNode? FirstNestedModel(JsonObject model)
    {
        foreach (var key in new[] { "entries", "cases" })
        {
            if (model[key] is not JsonArray entries)
                continue;
            foreach (var entry in entries)
                if (entry is JsonObject entryObject && entryObject["model"] is JsonNode nested)
                    return nested;
        }
        return null;
    }

    private static JsonNode? SelectGuiModel(JsonObject model)
    {
        if (model["cases"] is not JsonArray cases)
            return null;
        foreach (var entry in cases.OfType<JsonObject>())
        {
            var matchesGui = entry["when"] switch
            {
                JsonValue value when value.TryGetValue<string>(out var context) => context == "gui",
                JsonArray contexts => contexts.Any(context => context?.GetValue<string>() == "gui"),
                _ => false,
            };
            if (matchesGui)
                return entry["model"];
        }
        return null;
    }

    private static List<Pixel?> ReadTints(JsonNode? node)
    {
        var result = new List<Pixel?>();
        if (node is not JsonArray tints)
            return result;
        foreach (var tint in tints)
        {
            var colorNode = tint is JsonObject tintObject
                ? tintObject["value"] ?? tintObject["default"]
                : tint;
            var color = ReadColor(colorNode);
            result.Add(color);
        }
        return result;
    }

    private static Pixel? ReadColor(JsonNode? node)
    {
        if (node is JsonValue value)
        {
            if (value.TryGetValue<int>(out var rgb))
                return FromRgb(rgb);
            if (value.TryGetValue<string>(out var text))
            {
                text = text.TrimStart('#');
                if (int.TryParse(text, System.Globalization.NumberStyles.HexNumber, null, out rgb))
                    return FromRgb(rgb);
            }
        }
        if (node is JsonArray array && array.Count >= 3
            && array[0] is not null && array[1] is not null && array[2] is not null)
        {
            return new Pixel(
                (byte)Math.Clamp(array[0]!.GetValue<int>(), 0, 255),
                (byte)Math.Clamp(array[1]!.GetValue<int>(), 0, 255),
                (byte)Math.Clamp(array[2]!.GetValue<int>(), 0, 255), 255);
        }
        return null;
    }

    private static Pixel FromRgb(int rgb)
        => new((byte)(rgb >> 16), (byte)(rgb >> 8), (byte)rgb, 255);

    private static ResolvedModel? ResolveModel(string reference, AssetSet assets, HashSet<string> visiting)
    {
        if (!TryResourceId(reference, "minecraft", out var modelNamespace, out var modelPath))
            return null;
        var id = $"{modelNamespace}:{modelPath}";
        if (!visiting.Add(id))
            return null;
        try
        {
            if (id == "minecraft:builtin/generated")
                return new ResolvedModel { Generated = true };
            if (!assets.Models.TryGetValue(id, out var json))
                return null;

            ModelDocument? document;
            try
            {
                document = JsonSerializer.Deserialize<ModelDocument>(json, JsonOptions);
            }
            catch (JsonException)
            {
                return null;
            }
            if (document is null)
                return null;

            var resolved = string.IsNullOrWhiteSpace(document.Parent)
                ? new ResolvedModel()
                : ResolveModel(Qualify(document.Parent, "minecraft"), assets, visiting) ?? new ResolvedModel();
            resolved = resolved.Clone();
            foreach (var (key, texture) in document.Textures)
                resolved.Textures[key] = texture;
            if (document.Elements is not null)
                resolved.Elements = document.Elements;
            if (document.Display is not null)
                foreach (var (context, transform) in document.Display)
                    resolved.Display[context] = transform;
            if (document.GuiLight is not null)
                resolved.GuiLight = document.GuiLight;
            if (document.AmbientOcclusion.HasValue)
                resolved.AmbientOcclusion = document.AmbientOcclusion.Value;
            resolved.Generated |= document.Parent is "builtin/generated" or "minecraft:builtin/generated"
                or "item/generated" or "minecraft:item/generated";
            return resolved;
        }
        finally
        {
            visiting.Remove(id);
        }
    }

    private static void AddGeneratedModel(List<RenderQuad> quads, ResolvedModel model, IReadOnlyList<Pixel?> tints, AssetSet assets)
    {
        var transform = model.GuiTransform;
        for (var layer = 0; ; layer++)
        {
            if (!model.Textures.TryGetValue($"layer{layer}", out var reference))
            {
                if (layer == 0)
                    continue;
                break;
            }
            var texture = assets.Texture(ResolveTexture(reference, model.Textures));
            if (texture is null)
                continue;
            var points = new[] { V(0, 16, 8), V(16, 16, 8), V(16, 0, 8), V(0, 0, 8) };
            var vertices = new RenderVertex[4];
            var uvs = new[] { new Vec2(0, 0), new Vec2(16, 0), new Vec2(16, 16), new Vec2(0, 16) };
            for (var index = 0; index < 4; index++)
                vertices[index] = new RenderVertex(ApplyGuiTransform(points[index], transform), uvs[index].X, uvs[index].Y);
            quads.Add(new RenderQuad(vertices, texture, 1, Tint(tints, layer)));
        }
    }

    private static void AddBlockModel(List<RenderQuad> quads, ResolvedModel model, IReadOnlyList<Pixel?> tints, AssetSet assets)
    {
        var transform = model.GuiTransform;
        foreach (var element in model.Elements)
        {
            foreach (var (faceName, face) in element.Faces)
            {
                var texture = assets.Texture(ResolveTexture(face.Texture, model.Textures));
                var points = FacePoints(element.From, element.To, faceName);
                if (texture is null || points is null)
                    continue;

                var normal = FaceNormal(faceName);
                if (element.Rotation is not null)
                    normal = RotateAxis(normal, element.Rotation.Axis, element.Rotation.Angle);
                normal = ApplyGuiRotation(normal, transform);
                if (normal.Z <= Epsilon)
                    continue;

                var uv = face.Uv is { Length: 4 } ? face.Uv : DefaultFaceUv(element.From, element.To, faceName);
                var uvPoints = new[]
                {
                    new Vec2(uv[0], uv[1]), new Vec2(uv[2], uv[1]),
                    new Vec2(uv[2], uv[3]), new Vec2(uv[0], uv[3]),
                };
                var turns = ((face.Rotation / 90) % 4 + 4) % 4;
                if (turns > 0)
                    uvPoints = Enumerable.Range(0, 4).Select(index => uvPoints[(index + turns) % 4]).ToArray();

                var vertices = new RenderVertex[4];
                for (var index = 0; index < 4; index++)
                {
                    var point = element.Rotation is null ? points[index] : RotateElement(points[index], element.Rotation);
                    vertices[index] = new RenderVertex(ApplyGuiTransform(point, transform), uvPoints[index].X, uvPoints[index].Y);
                }
                var brightness = !element.Shade || string.Equals(model.GuiLight, "front", StringComparison.OrdinalIgnoreCase)
                    ? 1
                    : NormalBrightness(normal);
                quads.Add(new RenderQuad(vertices, texture, brightness, Tint(tints, face.TintIndex)));
            }
        }
    }

    private static void AddSpecialModel(
        List<RenderQuad> quads,
        ResolvedModel baseModel,
        JsonObject special,
        IReadOnlyList<Pixel?> tints,
        AssetSet assets)
    {
        var type = special["type"]?.GetValue<string>();
        if (type != "minecraft:chest")
            return;
        var textureRef = special["texture"]?.GetValue<string>();
        if (!TryResourceId(textureRef ?? "", "minecraft", out var textureNamespace, out var texturePath))
            return;
        var texture = assets.Texture($"{textureNamespace}:entity/chest/{texturePath}");
        if (texture is null)
            return;

        var model = baseModel.Clone();
        model.Textures["special"] = $"{textureNamespace}:entity/chest/{texturePath}";
        model.Elements =
        [
            Box([1, 1, 1], [15, 11, 15],
                [10.5, 8.25, 14, 10.75], [3.5, 8.25, 7, 10.75],
                [0, 8.25, 3.5, 10.75], [7, 8.25, 10.5, 10.75],
                [3.5, 4.75, 7, 8.25], [7, 4.75, 10.5, 8.25]),
            Box([1, 11, 1], [15, 16, 15],
                [10.5, 3.5, 14, 4.75], [3.5, 3.5, 7, 4.75],
                [0, 3.5, 3.5, 4.75], [7, 3.5, 10.5, 4.75],
                [3.5, 0, 7, 3.5], [7, 0, 10.5, 3.5]),
            new ModelElement
            {
                From = [7, 8, 15], To = [9, 12, 16],
                Faces = new(StringComparer.OrdinalIgnoreCase)
                {
                    ["south"] = new() { Texture = "#special", Uv = [0, 0, 0.5, 1] },
                },
            },
        ];
        AddBlockModel(quads, model, tints, assets);
    }

    private static ModelElement Box(
        double[] from, double[] to,
        double[] north, double[] south, double[] west, double[] east, double[] up, double[] down)
        => new()
        {
            From = from,
            To = to,
            Faces = new(StringComparer.OrdinalIgnoreCase)
            {
                ["north"] = new() { Texture = "#special", Uv = north },
                ["south"] = new() { Texture = "#special", Uv = south },
                ["west"] = new() { Texture = "#special", Uv = west },
                ["east"] = new() { Texture = "#special", Uv = east },
                ["up"] = new() { Texture = "#special", Uv = up },
                ["down"] = new() { Texture = "#special", Uv = down },
            },
        };

    private static byte[] Rasterize(IReadOnlyList<RenderQuad> quads)
    {
        const double pixelsPerModelUnit = 4;
        const double center = Size / 2d;
        var output = new byte[Size * Size * 4];
        var depth = Enumerable.Repeat(double.NegativeInfinity, Size * Size).ToArray();
        foreach (var quad in quads)
        {
            var vertices = quad.Vertices.Select(vertex => vertex with
            {
                Point = V(center + vertex.Point.X * pixelsPerModelUnit, center - vertex.Point.Y * pixelsPerModelUnit, vertex.Point.Z),
            }).ToArray();
            RasterizeTriangle(output, depth, vertices[0], vertices[1], vertices[2], quad);
            RasterizeTriangle(output, depth, vertices[0], vertices[2], vertices[3], quad);
        }
        return output;
    }

    private static void RasterizeTriangle(byte[] output, double[] depth, RenderVertex a, RenderVertex b, RenderVertex c, RenderQuad quad)
    {
        var minX = Math.Max(0, (int)Math.Floor(Math.Min(a.Point.X, Math.Min(b.Point.X, c.Point.X))));
        var maxX = Math.Min(Size - 1, (int)Math.Ceiling(Math.Max(a.Point.X, Math.Max(b.Point.X, c.Point.X))));
        var minY = Math.Max(0, (int)Math.Floor(Math.Min(a.Point.Y, Math.Min(b.Point.Y, c.Point.Y))));
        var maxY = Math.Min(Size - 1, (int)Math.Ceiling(Math.Max(a.Point.Y, Math.Max(b.Point.Y, c.Point.Y))));
        var denominator = (b.Point.Y - c.Point.Y) * (a.Point.X - c.Point.X) + (c.Point.X - b.Point.X) * (a.Point.Y - c.Point.Y);
        if (Math.Abs(denominator) < Epsilon)
            return;

        for (var y = minY; y <= maxY; y++)
        for (var x = minX; x <= maxX; x++)
        {
            var px = x + 0.5;
            var py = y + 0.5;
            var w1 = ((b.Point.Y - c.Point.Y) * (px - c.Point.X) + (c.Point.X - b.Point.X) * (py - c.Point.Y)) / denominator;
            var w2 = ((c.Point.Y - a.Point.Y) * (px - c.Point.X) + (a.Point.X - c.Point.X) * (py - c.Point.Y)) / denominator;
            var w3 = 1 - w1 - w2;
            if (w1 < -0.001 || w2 < -0.001 || w3 < -0.001)
                continue;
            var z = w1 * a.Point.Z + w2 * b.Point.Z + w3 * c.Point.Z;
            var pixelIndex = y * Size + x;
            if (z < depth[pixelIndex])
                continue;
            var u = w1 * a.U + w2 * b.U + w3 * c.U;
            var v = w1 * a.V + w2 * b.V + w3 * c.V;
            var color = quad.Texture.Pixel(
                Math.Clamp((int)(u / 16 * quad.Texture.FrameSize), 0, quad.Texture.FrameSize - 1),
                Math.Clamp((int)(v / 16 * quad.Texture.FrameSize), 0, quad.Texture.FrameSize - 1));
            if (color.A == 0)
                continue;
            if (quad.Tint is Pixel tint)
                color = new Pixel(
                    (byte)(color.R * tint.R / 255), (byte)(color.G * tint.G / 255),
                    (byte)(color.B * tint.B / 255), color.A);
            color = new Pixel(
                (byte)(color.R * quad.Brightness), (byte)(color.G * quad.Brightness),
                (byte)(color.B * quad.Brightness), color.A);
            Blend(output, pixelIndex * 4, color);
            depth[pixelIndex] = z;
        }
    }

    private static Vec3[]? FacePoints(double[] from, double[] to, string face) => face switch
    {
        "north" => [V(from[0], to[1], from[2]), V(to[0], to[1], from[2]), V(to[0], from[1], from[2]), V(from[0], from[1], from[2])],
        "south" => [V(to[0], to[1], to[2]), V(from[0], to[1], to[2]), V(from[0], from[1], to[2]), V(to[0], from[1], to[2])],
        "west" => [V(from[0], to[1], to[2]), V(from[0], to[1], from[2]), V(from[0], from[1], from[2]), V(from[0], from[1], to[2])],
        "east" => [V(to[0], to[1], from[2]), V(to[0], to[1], to[2]), V(to[0], from[1], to[2]), V(to[0], from[1], from[2])],
        "up" => [V(from[0], to[1], from[2]), V(from[0], to[1], to[2]), V(to[0], to[1], to[2]), V(to[0], to[1], from[2])],
        "down" => [V(from[0], from[1], to[2]), V(from[0], from[1], from[2]), V(to[0], from[1], from[2]), V(to[0], from[1], to[2])],
        _ => null,
    };

    private static double[] DefaultFaceUv(double[] from, double[] to, string face) => face switch
    {
        "north" or "south" => [from[0], 16 - to[1], to[0], 16 - from[1]],
        "west" or "east" => [from[2], 16 - to[1], to[2], 16 - from[1]],
        "up" or "down" => [from[0], from[2], to[0], to[2]],
        _ => [0, 0, 16, 16],
    };

    private static Vec3 RotateElement(Vec3 point, ModelRotation rotation)
    {
        var origin = V(rotation.Origin[0], rotation.Origin[1], rotation.Origin[2]);
        var rotated = RotateAxis(point - origin, rotation.Axis, rotation.Angle);
        if (rotation.Rescale)
        {
            var factor = 1 / Math.Cos(Math.Abs(rotation.Angle) * Math.PI / 180);
            rotated = rotation.Axis switch
            {
                "x" => V(rotated.X, rotated.Y * factor, rotated.Z * factor),
                "y" => V(rotated.X * factor, rotated.Y, rotated.Z * factor),
                "z" => V(rotated.X * factor, rotated.Y * factor, rotated.Z),
                _ => rotated,
            };
        }
        return rotated + origin;
    }

    private static Vec3 ApplyGuiTransform(Vec3 point, ModelTransform transform)
    {
        point -= V(8, 8, 8);
        point = V(point.X * transform.ScaleAt(0), point.Y * transform.ScaleAt(1), point.Z * transform.ScaleAt(2));
        point = RotateAxis(point, "z", transform.RotationAt(2));
        point = RotateAxis(point, "y", transform.RotationAt(1));
        point = RotateAxis(point, "x", transform.RotationAt(0));
        return point + V(transform.TranslationAt(0), transform.TranslationAt(1), transform.TranslationAt(2));
    }

    private static Vec3 ApplyGuiRotation(Vec3 vector, ModelTransform transform)
    {
        vector = RotateAxis(vector, "z", transform.RotationAt(2));
        vector = RotateAxis(vector, "y", transform.RotationAt(1));
        return RotateAxis(vector, "x", transform.RotationAt(0));
    }

    private static Vec3 RotateAxis(Vec3 point, string axis, double degrees)
    {
        var radians = degrees * Math.PI / 180;
        var sine = Math.Sin(radians);
        var cosine = Math.Cos(radians);
        return axis switch
        {
            "x" => V(point.X, point.Y * cosine - point.Z * sine, point.Y * sine + point.Z * cosine),
            "y" => V(point.X * cosine + point.Z * sine, point.Y, -point.X * sine + point.Z * cosine),
            "z" => V(point.X * cosine - point.Y * sine, point.X * sine + point.Y * cosine, point.Z),
            _ => point,
        };
    }

    private static string ResolveTexture(string reference, IReadOnlyDictionary<string, string> textures)
    {
        var seen = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
        while (reference.StartsWith('#') && seen.Add(reference))
            reference = textures.GetValueOrDefault(reference[1..], "");
        return reference;
    }

    private static Pixel? Tint(IReadOnlyList<Pixel?> tints, int index)
        => index >= 0 && index < tints.Count ? tints[index] : null;

    private static Vec3 FaceNormal(string face) => face switch
    {
        "north" => V(0, 0, -1), "south" => V(0, 0, 1),
        "west" => V(-1, 0, 0), "east" => V(1, 0, 0),
        "up" => V(0, 1, 0), "down" => V(0, -1, 0), _ => V(0, 0, 0),
    };

    private static double NormalBrightness(Vec3 normal)
    {
        var length = Math.Sqrt(normal.X * normal.X + normal.Y * normal.Y + normal.Z * normal.Z);
        if (length <= Epsilon)
            return 0.8;
        normal = V(normal.X / length, normal.Y / length, normal.Z / length);
        var light = V(-0.35, 0.8, 0.48);
        var dot = (normal.X * light.X + normal.Y * light.Y + normal.Z * light.Z)
            / Math.Sqrt(light.X * light.X + light.Y * light.Y + light.Z * light.Z);
        return Math.Clamp(0.62 + Math.Max(0, dot) * 0.38, 0.55, 1);
    }

    private static void Blend(byte[] target, int index, Pixel source)
    {
        if (source.A == 0)
            return;
        var alpha = source.A / 255d;
        target[index] = (byte)(source.B * alpha + target[index] * (1 - alpha));
        target[index + 1] = (byte)(source.G * alpha + target[index + 1] * (1 - alpha));
        target[index + 2] = (byte)(source.R * alpha + target[index + 2] * (1 - alpha));
        target[index + 3] = (byte)Math.Min(255, source.A + target[index + 3] * (1 - alpha));
    }

    private static string Qualify(string value, string defaultNamespace)
        => value.Contains(':') ? value : $"{defaultNamespace}:{value}";

    private static bool TryResourceId(string value, string defaultNamespace, out string resourceNamespace, out string path)
    {
        value = value.Trim().TrimStart('/');
        var separator = value.IndexOf(':');
        resourceNamespace = separator >= 0 ? value[..separator] : defaultNamespace;
        path = separator >= 0 ? value[(separator + 1)..] : value;
        return resourceNamespace.Length > 0 && path.Length > 0
            && !resourceNamespace.Contains("..", StringComparison.Ordinal)
            && !path.Contains("..", StringComparison.Ordinal);
    }

    private static Vec3 V(double x, double y, double z) => new(x, y, z);

    private sealed class AssetSet
    {
        public Dictionary<string, string> Json { get; } = new(StringComparer.OrdinalIgnoreCase);
        public Dictionary<string, string> Models { get; } = new(StringComparer.OrdinalIgnoreCase);
        private Dictionary<string, TextureData> Textures { get; } = new(StringComparer.OrdinalIgnoreCase);

        public async Task AddAsync(PlayerItemAsset asset)
        {
            var path = asset.Path.Replace('\\', '/').TrimStart('/').ToLowerInvariant();
            if (path.EndsWith(".json", StringComparison.Ordinal))
            {
                var json = asset.Data.ToStringUtf8();
                Json[path] = json;
                if (TryPathId(path, "models", ".json", out var id))
                    Models[id] = json;
            }
            else if (path.EndsWith(".png", StringComparison.Ordinal)
                && TryPathId(path, "textures", ".png", out var id))
            {
                var texture = await DecodeTextureAsync(asset.Data.ToByteArray());
                if (texture is not null)
                    Textures[id] = texture;
            }
        }

        public TextureData? Texture(string id)
            => Textures.GetValueOrDefault(id.Contains(':') ? id : $"minecraft:{id}");

        private static bool TryPathId(string path, string kind, string suffix, out string id)
        {
            id = "";
            var segments = path.Split('/', StringSplitOptions.RemoveEmptyEntries);
            if (segments.Length < 4 || segments[0] != "assets" || segments[2] != kind || !path.EndsWith(suffix))
                return false;
            var prefix = $"assets/{segments[1]}/{kind}/";
            id = $"{segments[1]}:{path[prefix.Length..^suffix.Length]}";
            return true;
        }
    }

    private static async Task<TextureData?> DecodeTextureAsync(byte[] png)
    {
        if (png.Length == 0)
            return null;
        try
        {
            using var stream = new InMemoryRandomAccessStream();
            using (var writer = new DataWriter(stream))
            {
                writer.WriteBytes(png);
                await writer.StoreAsync();
                writer.DetachStream();
            }
            stream.Seek(0);
            var decoder = await BitmapDecoder.CreateAsync(stream);
            var provider = await decoder.GetPixelDataAsync(
                BitmapPixelFormat.Rgba8, BitmapAlphaMode.Straight, new BitmapTransform(),
                ExifOrientationMode.IgnoreExifOrientation, ColorManagementMode.DoNotColorManage);
            return new TextureData((int)decoder.PixelWidth, (int)decoder.PixelHeight, provider.DetachPixelData());
        }
        catch
        {
            return null;
        }
    }

    private sealed class ModelDocument
    {
        public string Parent { get; set; } = "";
        public bool? AmbientOcclusion { get; set; }
        public string? GuiLight { get; set; }
        public Dictionary<string, string> Textures { get; set; } = new(StringComparer.OrdinalIgnoreCase);
        public List<ModelElement>? Elements { get; set; }
        public Dictionary<string, ModelTransform>? Display { get; set; }
    }

    private sealed class ResolvedModel
    {
        public bool Generated { get; set; }
        public bool AmbientOcclusion { get; set; } = true;
        public string GuiLight { get; set; } = "side";
        public Dictionary<string, string> Textures { get; set; } = new(StringComparer.OrdinalIgnoreCase);
        public List<ModelElement> Elements { get; set; } = [];
        public Dictionary<string, ModelTransform> Display { get; set; } = new(StringComparer.OrdinalIgnoreCase);
        public ModelTransform GuiTransform => Display.GetValueOrDefault("gui") ?? new();

        public ResolvedModel Clone() => new()
        {
            Generated = Generated,
            AmbientOcclusion = AmbientOcclusion,
            GuiLight = GuiLight,
            Textures = new Dictionary<string, string>(Textures, StringComparer.OrdinalIgnoreCase),
            Elements = Elements,
            Display = new Dictionary<string, ModelTransform>(Display, StringComparer.OrdinalIgnoreCase),
        };
    }

    private sealed class ModelElement
    {
        public double[] From { get; set; } = [0, 0, 0];
        public double[] To { get; set; } = [16, 16, 16];
        public ModelRotation? Rotation { get; set; }
        public bool Shade { get; set; } = true;
        public Dictionary<string, ModelFace> Faces { get; set; } = new(StringComparer.OrdinalIgnoreCase);
    }

    private sealed class ModelRotation
    {
        public double[] Origin { get; set; } = [8, 8, 8];
        public string Axis { get; set; } = "";
        public double Angle { get; set; }
        public bool Rescale { get; set; }
    }

    private sealed class ModelFace
    {
        public double[]? Uv { get; set; }
        public string Texture { get; set; } = "";
        public int Rotation { get; set; }
        public int TintIndex { get; set; } = -1;
    }

    private sealed class ModelTransform
    {
        public double[]? Rotation { get; set; }
        public double[]? Translation { get; set; }
        public double[]? Scale { get; set; }
        public double RotationAt(int index) => Rotation is { Length: > 2 } ? Rotation[index] : 0;
        public double TranslationAt(int index) => Translation is { Length: > 2 } ? Math.Clamp(Translation[index], -80, 80) : 0;
        public double ScaleAt(int index) => Scale is { Length: > 2 } ? Math.Min(Scale[index], 4) : 1;
    }

    private sealed class TextureData(int width, int height, byte[] pixels)
    {
        public int FrameSize { get; } = Math.Min(width, height);
        public Pixel Pixel(int x, int y)
        {
            var index = (Math.Clamp(y, 0, height - 1) * width + Math.Clamp(x, 0, width - 1)) * 4;
            return new Pixel(pixels[index], pixels[index + 1], pixels[index + 2], pixels[index + 3]);
        }
    }

    private readonly record struct Pixel(byte R, byte G, byte B, byte A);
    private readonly record struct Vec2(double X, double Y);
    private readonly record struct Vec3(double X, double Y, double Z)
    {
        public static Vec3 operator +(Vec3 left, Vec3 right) => V(left.X + right.X, left.Y + right.Y, left.Z + right.Z);
        public static Vec3 operator -(Vec3 left, Vec3 right) => V(left.X - right.X, left.Y - right.Y, left.Z - right.Z);
    }
    private readonly record struct RenderVertex(Vec3 Point, double U, double V);
    private sealed record RenderQuad(RenderVertex[] Vertices, TextureData Texture, double Brightness, Pixel? Tint);
    private sealed record ItemPart(string Model, List<Pixel?> Tints, JsonObject? Special);
}
