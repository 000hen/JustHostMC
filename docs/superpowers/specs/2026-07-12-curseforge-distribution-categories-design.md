# CurseForge Distribution and Categories Design

## Goal

Respect each shop project's distribution policy before the user starts an
install, provide a source-driven browser action for projects that require a
website download, and expose correct source-native CurseForge categories with
localization and disk-backed caching.

## Scope

This change extends the generic Lua shop contract, the gRPC contract, and the
WinUI Mod Shop. CurseForge is the first shop to provide dynamic categories and
project distribution policy. Existing third-party shop scripts remain
compatible because the new script fields and `categories(ctx)` entry point are
optional.

## Contract

Add `ShopDistribution` with three values:

- `SHOP_DISTRIBUTION_UNKNOWN`: the source did not declare a policy; retain the
  existing guarded install behavior.
- `SHOP_DISTRIBUTION_DIRECT`: the source permits an API download.
- `SHOP_DISTRIBUTION_WEBSITE_ONLY`: the source requires the user to obtain the
  file on its website.

Add `distribution` to `ShopProject`. A shop script returns this as
`distribution = "direct" | "website_only"`; a missing value maps to unknown.

Add an optional `categories(ctx)` shop function and a corresponding
`ShopService.GetCategories` RPC. The request contains `shop_id` and the current
`ModKind`. Each returned category contains:

- the source-native ID used by search;
- the upstream display name used as a fallback;
- an optional localization key;
- an optional source-native slug, for diagnostics and resource naming.

If a user-provided shop omits `categories`, the engine returns an empty list.

## CurseForge Mapping

The CurseForge project field `allowModDistribution` maps as follows:

- `false` -> website-only;
- `true` -> direct;
- `null` or absent -> unknown.

The existing `resolve_file` behavior remains the final enforcement boundary. It
must still reject a missing `downloadUrl` when the dedicated download URL
endpoint cannot provide one, and `ShopService.Install` must return before
calling the downloader.

CurseForge categories are fetched from `/v1/categories?gameId=432&classId=N`,
where `N` is the Mods class for `MOD` context and the Bukkit Plugins class for
`PLUGIN` context. Only non-class child categories belonging to that class are
returned. Their numeric IDs are submitted to `/v1/mods/search` through the
`categoryIds` query parameter. The existing `classId` continues to select the
section.

## Cache Policy

Category requests use `jhmc.http_cache` so they share the engine's disk-backed
ETag cache. A category response is fresh for 24 hours. After that window the
cache revalidates with the upstream ETag and reuses the stored body on HTTP 304.
The cache key includes the request URL, so Mods and Plugins category sets remain
independent.

Search and detail cache windows remain unchanged.

## Localization

CurseForge category localization keys use
`shop.category.curseforge.<slug>`. Known current slugs receive en-US and
Traditional Chinese resources. When a new upstream category has no resource,
the frontend detects the unresolved key and displays the upstream category name
instead.

The website-only action label is generic and source-driven. The localized
template is `Get on {source}` in English, with an equivalent Traditional Chinese
resource. `{source}` is the selected shop script's runtime `ShopInfo.name`; the
UI must not compare or hard-code the shop ID `curseforge`.

## UI Behavior

The detail view derives one primary action for both the latest-release button
and every version row:

- direct or unknown: show the existing localized Install label and execute the
  dependency/install flow;
- website-only: show the localized `Get on {source}` label and launch the
  project's website URL in the default browser.

The browser path must not show the dependency dialog or invoke
`ShopService.Install`. The action remains disabled until a valid project website
URL is available. The existing secondary Open website link remains available.

Project cards carry distribution state from search/home results when the source
provides it. Project detail refreshes the state and supplies the canonical
website URL.

## Error Handling

The engine continues returning the typed `SHOP_FILE_NOT_DISTRIBUTABLE` error for
policy or per-file failures that reach install. This protects older clients,
unknown policies, and upstream inconsistencies. The frontend retains the
localized fallback error message.

Category load failures do not break browsing or search. The category filter is
left empty and the existing localized shop-load status is shown where
appropriate.

## Testing

Use red-green TDD for hand-written behavior:

1. Lua shop bridge tests cover optional categories, category field mapping, and
   distribution parsing.
2. CurseForge HTTP fixture tests cover class filtering, numeric category IDs,
   `categoryIds`, localization keys, the 24-hour cache path, and all three
   `allowModDistribution` states.
3. gRPC service tests cover category mapping and project distribution mapping.
4. Frontend tests cover primary action selection, dynamic source-name formatting,
   localization fallback, and browser-only actions not entering install.
5. Regenerate protobuf outputs, then run focused Go tests, .NET tests, and the
   platform-specific solution build.

Generated Go and C# protobuf files are build artifacts and are not committed.

## Out of Scope

- Downloading a website-only file on the user's behalf.
- Scraping CurseForge web pages.
- Replacing Modrinth's existing category list in this change.
- Changing the existing install checksum or dependency workflow.
