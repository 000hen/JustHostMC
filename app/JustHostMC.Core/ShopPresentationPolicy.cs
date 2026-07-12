using McManager.Grpc;

namespace JustHostMC.Core;

public enum ShopPrimaryActionKind {
    Install,
    Website,
}

public readonly record struct ShopPrimaryAction(ShopPrimaryActionKind Kind,
                                                bool IsEnabled);

public static class ShopPresentationPolicy {
    public static ShopPrimaryAction DeterminePrimaryAction(
        ShopDistribution distribution, string websiteUrl) {
        if (distribution != ShopDistribution.WebsiteOnly)
            return new ShopPrimaryAction(ShopPrimaryActionKind.Install, true);

        var valid = Uri.TryCreate(websiteUrl, UriKind.Absolute, out var uri) &&
                    (uri.Scheme == Uri.UriSchemeHttps ||
                     uri.Scheme == Uri.UriSchemeHttp);
        return new ShopPrimaryAction(ShopPrimaryActionKind.Website, valid);
    }

    public static string ResolveCategoryLabel(
        ShopCategory category, Func<string, string> resolve) {
        if (category.LocalizationKey.Length == 0)
            return category.Name;

        var localized = resolve(category.LocalizationKey);
        return localized == category.LocalizationKey ? category.Name : localized;
    }
}
