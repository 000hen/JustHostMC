using System.Runtime.InteropServices.WindowsRuntime;
using System.Text.Json;
using Google.Protobuf;
using McManager.Grpc;
using Microsoft.UI.Xaml.Media;
using Microsoft.UI.Xaml.Media.Imaging;
using Windows.Graphics.Imaging;
using Windows.Storage.Streams;

namespace JustHostMC.App.Services;

/// <summary>Renders flattened Minecraft item/block models using local texture bytes.</summary>
internal static class MinecraftItemIconRenderer
{
    private const int Size = 64;
    private static readonly JsonSerializerOptions JsonOptions = new() { PropertyNameCaseInsensitive = true };
    // Default foliage tint applied to faces with a tintindex (leaves, grass, ferns, etc.).
    private static readonly Pixel DefaultFoliageTint = new(72, 181, 24, 255);

    public static async Task<ImageSource?> RenderAsync(string modelJson, IEnumerable<PlayerItemTexture> textureMessages)
    {
        if (string.IsNullOrWhiteSpace(modelJson))
            return null;
        ModelAsset? model;
        try
        {
            model = JsonSerializer.Deserialize<ModelAsset>(modelJson, JsonOptions);
        }
        catch (JsonException)
        {
            return null;
        }
        if (model is null)
            return null;

        var textures = new Dictionary<string, TextureData>(StringComparer.OrdinalIgnoreCase);
        foreach (var message in textureMessages)
        {
            var decoded = await DecodeTextureAsync(message.Png);
            if (decoded is not null)
                textures[message.Id] = decoded;
        }

        var pixels = model.Special == "chest"
            ? RenderSpecialChest(model, textures)
            : model.Elements is { Count: > 0 }
                ? RenderBlockModel(model, textures)
                : RenderGeneratedItem(model, textures);
        if (pixels is null)
            return null;

        var bitmap = new WriteableBitmap(Size, Size);
        using (var stream = bitmap.PixelBuffer.AsStream())
        {
            stream.Write(pixels, 0, pixels.Length);
        }
        bitmap.Invalidate();
        return bitmap;
    }

    private static async Task<TextureData?> DecodeTextureAsync(ByteString png)
    {
        if (png.IsEmpty)
            return null;
        try
        {
            using var stream = new InMemoryRandomAccessStream();
            using (var writer = new DataWriter(stream))
            {
                writer.WriteBytes(png.ToByteArray());
                await writer.StoreAsync();
                writer.DetachStream();
            }
            stream.Seek(0);
            var decoder = await BitmapDecoder.CreateAsync(stream);
            var provider = await decoder.GetPixelDataAsync(
                BitmapPixelFormat.Rgba8,
                BitmapAlphaMode.Straight,
                new BitmapTransform(),
                ExifOrientationMode.IgnoreExifOrientation,
                ColorManagementMode.DoNotColorManage);
            return new TextureData((int)decoder.PixelWidth, (int)decoder.PixelHeight, provider.DetachPixelData());
        }
        catch
        {
            return null;
        }
    }

    private static byte[]? RenderGeneratedItem(ModelAsset model, IReadOnlyDictionary<string, TextureData> textures)
    {
        var output = new byte[Size * Size * 4];
        var found = false;
        for (var layer = 0; layer < 8; layer++)
        {
            if (!model.Textures.TryGetValue($"layer{layer}", out var reference))
                continue;
            reference = ResolveTexture(reference, model.Textures);
            if (!textures.TryGetValue(reference, out var texture))
                continue;
            found = true;
            bool applyTint = model.LayerTints?.Contains(layer) == true;
            for (var y = 4; y < Size - 4; y++)
            {
                for (var x = 4; x < Size - 4; x++)
                {
                    var sourceX = (x - 4) * texture.FrameSize / (Size - 8);
                    var sourceY = (y - 4) * texture.FrameSize / (Size - 8);
                    var pixel = texture.Pixel(sourceX, sourceY);
                    if (applyTint && pixel.A > 0)
                    {
                        pixel = new Pixel(
                            (byte)(pixel.R * DefaultFoliageTint.R / 255),
                            (byte)(pixel.G * DefaultFoliageTint.G / 255),
                            (byte)(pixel.B * DefaultFoliageTint.B / 255),
                            pixel.A);
                    }
                    Blend(output, (y * Size + x) * 4, pixel);
                }
            }
        }
        return found ? output : null;
    }

    private static byte[]? RenderBlockModel(ModelAsset model, IReadOnlyDictionary<string, TextureData> textures)
    {
        var quads = new List<RenderQuad>();
        foreach (var element in model.Elements!)
        {
            foreach (var (faceName, face) in element.Faces)
            {
                var reference = ResolveTexture(face.Texture, model.Textures);
                if (!textures.TryGetValue(reference, out var texture))
                    continue;
                var points = FacePoints(element.From, element.To, faceName);
                if (points is null)
                    continue;
                var normal = FaceNormal(faceName);
                if (element.Rotation is not null)
                    normal = RotateAxis(normal, element.Rotation.Axis, element.Rotation.Angle);
                normal = ApplyGuiRotation(normal, model.Gui);
                // The item camera looks down -Z. Without backface culling,
                // zero-thickness planes let mirrored faces overwrite each other
                // according to dictionary enumeration order.
                if (normal.Z <= 0.000001)
                    continue;
                var uv = face.Uv is { Length: 4 } && face.Uv.Any(value => value != 0)
                    ? face.Uv
                    : DefaultFaceUv(element.From, element.To, faceName);
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
                    var point = points[index];
                    if (element.Rotation is not null)
                        point = RotateElement(point, element.Rotation);
                    point = ApplyGuiTransform(point, model.Gui);
                    vertices[index] = new RenderVertex(point, uvPoints[index].X, uvPoints[index].Y);
                }
                quads.Add(new RenderQuad(vertices, texture, NormalBrightness(normal), face.TintIndex));
            }
        }
        return quads.Count == 0 ? null : Rasterize(quads);
    }

    private static byte[] Rasterize(IReadOnlyList<RenderQuad> quads)
    {
        // Minecraft renders GUI models in a fixed orthographic frame. Auto-fitting
        // the geometry would erase the model's own display.scale and make thin
        // models (fences, gates, panes) unnaturally large.
        const double pixelsPerModelUnit = 4;
        const double center = Size / 2d;
        var output = new byte[Size * Size * 4];
        var depth = Enumerable.Repeat(double.NegativeInfinity, Size * Size).ToArray();

        foreach (var quad in quads)
        {
            var vertices = quad.Vertices.Select(vertex => vertex with
            {
                Point = new Vec3(
                    center + vertex.Point.X * pixelsPerModelUnit,
                    center - vertex.Point.Y * pixelsPerModelUnit,
                    vertex.Point.Z),
            }).ToArray();
            RasterizeTriangle(output, depth, vertices[0], vertices[1], vertices[2], quad.Texture, quad.Brightness, quad.TintIndex);
            RasterizeTriangle(output, depth, vertices[0], vertices[2], vertices[3], quad.Texture, quad.Brightness, quad.TintIndex);
        }
        return output;
    }

    private static void RasterizeTriangle(
        byte[] output, double[] depth, RenderVertex a, RenderVertex b, RenderVertex c,
        TextureData texture, double brightness, int tintIndex)
    {
        var minX = Math.Max(0, (int)Math.Floor(Math.Min(a.Point.X, Math.Min(b.Point.X, c.Point.X))));
        var maxX = Math.Min(Size - 1, (int)Math.Ceiling(Math.Max(a.Point.X, Math.Max(b.Point.X, c.Point.X))));
        var minY = Math.Max(0, (int)Math.Floor(Math.Min(a.Point.Y, Math.Min(b.Point.Y, c.Point.Y))));
        var maxY = Math.Min(Size - 1, (int)Math.Ceiling(Math.Max(a.Point.Y, Math.Max(b.Point.Y, c.Point.Y))));
        var denominator = (b.Point.Y - c.Point.Y) * (a.Point.X - c.Point.X)
            + (c.Point.X - b.Point.X) * (a.Point.Y - c.Point.Y);
        if (Math.Abs(denominator) < 0.000001)
            return;

        for (var y = minY; y <= maxY; y++)
        {
            for (var x = minX; x <= maxX; x++)
            {
                var px = x + 0.5;
                var py = y + 0.5;
                var w1 = ((b.Point.Y - c.Point.Y) * (px - c.Point.X)
                    + (c.Point.X - b.Point.X) * (py - c.Point.Y)) / denominator;
                var w2 = ((c.Point.Y - a.Point.Y) * (px - c.Point.X)
                    + (a.Point.X - c.Point.X) * (py - c.Point.Y)) / denominator;
                var w3 = 1 - w1 - w2;
                if (w1 < -0.001 || w2 < -0.001 || w3 < -0.001)
                    continue;
                var z = w1 * a.Point.Z + w2 * b.Point.Z + w3 * c.Point.Z;
                var pixelIndex = y * Size + x;
                if (z < depth[pixelIndex])
                    continue;
                var u = w1 * a.U + w2 * b.U + w3 * c.U;
                var v = w1 * a.V + w2 * b.V + w3 * c.V;
                var color = texture.Pixel(
                    Math.Clamp((int)(u / 16 * texture.FrameSize), 0, texture.FrameSize - 1),
                    Math.Clamp((int)(v / 16 * texture.FrameSize), 0, texture.FrameSize - 1));
                if (color.A == 0)
                    continue;
                if (tintIndex >= 0)
                    color = new Pixel(
                        (byte)(color.R * DefaultFoliageTint.R / 255),
                        (byte)(color.G * DefaultFoliageTint.G / 255),
                        (byte)(color.B * DefaultFoliageTint.B / 255),
                        color.A);
                var target = pixelIndex * 4;
                output[target] = (byte)(color.B * brightness);
                output[target + 1] = (byte)(color.G * brightness);
                output[target + 2] = (byte)(color.R * brightness);
                output[target + 3] = color.A;
                depth[pixelIndex] = z;
            }
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
        return rotated + origin;
    }

    private static Vec3 ApplyGuiTransform(Vec3 point, ModelTransform transform)
    {
        point -= V(8, 8, 8);
        var scale = transform.Scale is { Length: 3 } && transform.Scale.Any(value => value != 0)
            ? transform.Scale
            : [1d, 1d, 1d];
        point = V(point.X * scale[0], point.Y * scale[1], point.Z * scale[2]);
        // Minecraft composes its XYZ quaternion, so when it is applied to a
        // vertex the effective order is Z, then Y, then X.
        point = RotateAxis(point, "z", transform.RotationAt(2));
        point = RotateAxis(point, "y", transform.RotationAt(1));
        point = RotateAxis(point, "x", transform.RotationAt(0));
        return point + V(transform.TranslationAt(0), transform.TranslationAt(1), transform.TranslationAt(2));
    }

    private static byte[]? RenderSpecialChest(ModelAsset source, IReadOnlyDictionary<string, TextureData> textures)
    {
        if (!source.Textures.ContainsKey("special"))
            return null;
        var model = new ModelAsset
        {
            Gui = source.Gui,
            Textures = source.Textures,
            Elements =
            [
                ChestBox(
                    [1, 1, 1], [15, 11, 15],
                    north: [10.5, 8.25, 14, 10.75], south: [3.5, 8.25, 7, 10.75],
                    west: [0, 8.25, 3.5, 10.75], east: [7, 8.25, 10.5, 10.75],
                    up: [3.5, 4.75, 7, 8.25], down: [7, 4.75, 10.5, 8.25]),
                ChestBox(
                    [1, 11, 1], [15, 16, 15],
                    north: [10.5, 3.5, 14, 4.75], south: [3.5, 3.5, 7, 4.75],
                    west: [0, 3.5, 3.5, 4.75], east: [7, 3.5, 10.5, 4.75],
                    up: [3.5, 0, 7, 3.5], down: [7, 0, 10.5, 3.5]),
                new ModelElement
                {
                    From = [7, 8, 15],
                    To = [9, 12, 16],
                    Faces = new Dictionary<string, ModelFace>(StringComparer.OrdinalIgnoreCase)
                    {
                        ["south"] = new() { Texture = "#special", Uv = [0, 0, 0.5, 1] },
                    },
                },
            ],
        };
        return RenderBlockModel(model, textures);
    }

    private static ModelElement ChestBox(
        double[] from, double[] to,
        double[] north, double[] south, double[] west, double[] east, double[] up, double[] down)
        => new()
        {
            From = from,
            To = to,
            Faces = new Dictionary<string, ModelFace>(StringComparer.OrdinalIgnoreCase)
            {
                ["north"] = new() { Texture = "#special", Uv = north },
                ["south"] = new() { Texture = "#special", Uv = south },
                ["west"] = new() { Texture = "#special", Uv = west },
                ["east"] = new() { Texture = "#special", Uv = east },
                ["up"] = new() { Texture = "#special", Uv = up },
                ["down"] = new() { Texture = "#special", Uv = down },
            },
        };

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
        for (var index = 0; reference.StartsWith('#') && index < 16; index++)
            reference = textures.GetValueOrDefault(reference[1..], "");
        return reference;
    }

    private static Vec3 FaceNormal(string face) => face switch
    {
        "north" => V(0, 0, -1),
        "south" => V(0, 0, 1),
        "west" => V(-1, 0, 0),
        "east" => V(1, 0, 0),
        "up" => V(0, 1, 0),
        "down" => V(0, -1, 0),
        _ => V(0, 0, 0),
    };

    private static double NormalBrightness(Vec3 normal)
    {
        var length = Math.Sqrt(normal.X * normal.X + normal.Y * normal.Y + normal.Z * normal.Z);
        if (length <= 0.000001)
            return 0.8;
        normal = V(normal.X / length, normal.Y / length, normal.Z / length);
        var light = V(-0.35, 0.8, 0.48);
        var lightLength = Math.Sqrt(light.X * light.X + light.Y * light.Y + light.Z * light.Z);
        var dot = (normal.X * light.X + normal.Y * light.Y + normal.Z * light.Z) / lightLength;
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

    private static Vec3 V(double x, double y, double z) => new(x, y, z);

    private sealed class ModelAsset
    {
        public Dictionary<string, string> Textures { get; set; } = new(StringComparer.OrdinalIgnoreCase);
        public List<ModelElement>? Elements { get; set; }
        public ModelTransform Gui { get; set; } = new();
        public string Special { get; set; } = "";
        public List<int>? LayerTints { get; set; }
    }

    private sealed class ModelElement
    {
        public double[] From { get; set; } = [0, 0, 0];
        public double[] To { get; set; } = [16, 16, 16];
        public ModelRotation? Rotation { get; set; }
        public Dictionary<string, ModelFace> Faces { get; set; } = new(StringComparer.OrdinalIgnoreCase);
    }

    private sealed class ModelRotation
    {
        public double[] Origin { get; set; } = [8, 8, 8];
        public string Axis { get; set; } = "";
        public double Angle { get; set; }
    }

    private sealed class ModelFace
    {
        public double[] Uv { get; set; } = [0, 0, 0, 0];
        public string Texture { get; set; } = "";
        public int Rotation { get; set; }
        public int TintIndex { get; set; } = -1;
    }

    private sealed class ModelTransform
    {
        public double[] Rotation { get; set; } = [0, 0, 0];
        public double[] Translation { get; set; } = [0, 0, 0];
        public double[] Scale { get; set; } = [1, 1, 1];
        public double RotationAt(int index) => Rotation.Length > index ? Rotation[index] : 0;
        public double TranslationAt(int index) => Translation.Length > index ? Translation[index] : 0;
    }

    private sealed record TextureData(int Width, int Height, byte[] Pixels)
    {
        public int FrameSize => Math.Min(Width, Height);
        public Pixel Pixel(int x, int y)
        {
            var index = (y * Width + x) * 4;
            return new Pixel(Pixels[index], Pixels[index + 1], Pixels[index + 2], Pixels[index + 3]);
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
    private sealed record RenderQuad(RenderVertex[] Vertices, TextureData Texture, double Brightness, int TintIndex);
}
