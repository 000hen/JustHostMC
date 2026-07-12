# CurseForge Distribution and Categories Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make website-only CurseForge projects open their source website through a dynamic localized action, and provide correct cached CurseForge category filters.

**Architecture:** Extend the generic shop proto and optional Lua shop surface with project distribution metadata and category discovery. CurseForge maps `allowModDistribution`, fetches class-specific categories through `jhmc.http_cache`, and sends numeric category IDs back to search. A small pure C# presentation policy keeps WinUI action and localization-fallback decisions unit-testable.

**Tech Stack:** Protocol Buffers/buf, Go 1.26, gopher-lua, gRPC-Go, C#/.NET 9, WinUI 3, CommunityToolkit.Mvvm, xUnit, `.resw` resources.

## Global Constraints

- Preserve compatibility with shop scripts that omit `categories(ctx)` and `distribution`.
- `allowModDistribution == false` is website-only; `true` is direct; null/absent is unknown.
- The engine's existing `resolve_file` and `SHOP_FILE_NOT_DISTRIBUTABLE` path remains the final download-policy guard.
- Category requests use `jhmc.http_cache` with a 24-hour TTL and the existing disk ETag cache.
- Website action text is localized from `Get on {source}` using runtime `ShopInfo.name`; never compare the shop ID to `curseforge` in UI code.
- Unknown category localization keys fall back to the upstream name.
- WinUI builds always use an explicit platform (`x64` for verification).
- Prefix every shell command and every command-chain segment with `rtk`.
- Do not commit `engine/gen/` or `build/engine.exe`.

---

### Task 1: Extend the protobuf shop contract

**Files:**
- Modify: `proto/mcmanager/v1/mcmanager.proto:552-690`
- Modify: `proto/mcmanager/v1/mcmanager.proto:795-807`
- Generated, not committed: `engine/gen/mcmanager/v1/*`

**Interfaces:**
- Produces: `ShopDistribution`, `ShopCategory`, `ShopCategoriesRequest`, `ShopCategoryList`, `ShopProject.distribution`, and `ShopService.GetCategories`.
- Consumed by: Tasks 2, 4, 5, and 6.

- [ ] **Step 1: Add the additive enum, messages, field, and RPC**

Insert this enum before `ShopProject`:

```proto
enum ShopDistribution {
  SHOP_DISTRIBUTION_UNKNOWN = 0;
  SHOP_DISTRIBUTION_DIRECT = 1;
  SHOP_DISTRIBUTION_WEBSITE_ONLY = 2;
}
```

Append this field to `ShopProject` without renumbering existing fields:

```proto
  ShopDistribution distribution = 12;
```

Add these messages beside the other shop request/reply messages:

```proto
message ShopCategoriesRequest {
  string shop_id = 1;
  ModKind kind = 2;
}

message ShopCategory {
  string id = 1;
  string name = 2;
  string slug = 3;
  string localization_key = 4;
}

message ShopCategoryList {
  repeated ShopCategory categories = 1;
}
```

Add this RPC after `List`:

```proto
  rpc GetCategories(ShopCategoriesRequest) returns (ShopCategoryList);
```

- [ ] **Step 2: Validate and regenerate the contract**

Run from `proto/`:

```powershell
rtk buf lint
rtk buf generate
```

Expected: both commands exit 0; Go stubs include `GetCategories`; C# remains build-generated.

- [ ] **Step 3: Commit the contract checkpoint**

```powershell
rtk git add proto/mcmanager/v1/mcmanager.proto
rtk git commit -m feat:extend-shop-contract
```

---

### Task 2: Add optional categories and distribution to the Lua shop bridge

**Files:**
- Modify: `engine/internal/scripting/shop.go:29-126,190-249,427-443`
- Modify: `engine/internal/scripting/shop_test.go:11-155`

**Interfaces:**
- Consumes: `ShopDistribution` semantic strings `direct` and `website_only` from shop scripts.
- Produces: `ShopCategory`, `ShopProject.Distribution`, `LuaShop.Categories(context.Context, string)`, and optional Lua-function invocation.

- [ ] **Step 1: Write failing bridge tests**

Add a category-capable fixture and tests to `shop_test.go`:

```go
const categoryShopSrc = `
meta = { id = "category-shop", name = "Category Shop" }
function categories(ctx)
  return { categories = {
    { id = "12", name = "Technology", slug = "technology",
      localization_key = "shop.category.curseforge.technology" },
  } }
end
function search(ctx)
  return { projects = {{ project_id = "p", title = "P",
    distribution = "website_only" }}, total = 1 }
end
function home(ctx) return { sections = {} } end
function detail(ctx) return { project = { project_id = ctx.project_id } } end
function versions(ctx) return { versions = {} } end
function resolve_file(ctx) return { url = "https://example.test/x.jar", filename = "x.jar" } end
`

func TestShopCategoriesMapsOptionalFunction(t *testing.T) {
	ss := newTestShopSet(t, nil)
	shop, err := ss.AddSource(context.Background(), categoryShopSrc, true)
	if err != nil { t.Fatal(err) }
	categories, err := shop.Categories(context.Background(), "mod")
	if err != nil { t.Fatal(err) }
	want := ShopCategory{ID: "12", Name: "Technology", Slug: "technology",
		LocalizationKey: "shop.category.curseforge.technology"}
	if len(categories) != 1 || categories[0] != want {
		t.Fatalf("categories = %#v, want %#v", categories, want)
	}
}

func TestShopCategoriesMissingFunctionIsEmpty(t *testing.T) {
	ss := newTestShopSet(t, nil)
	shop, err := ss.AddSource(context.Background(), testShopSrc, true)
	if err != nil { t.Fatal(err) }
	categories, err := shop.Categories(context.Background(), "mod")
	if err != nil || len(categories) != 0 {
		t.Fatalf("categories = %#v, err = %v", categories, err)
	}
}

func TestShopProjectMapsDistribution(t *testing.T) {
	ss := newTestShopSet(t, nil)
	shop, err := ss.AddSource(context.Background(), categoryShopSrc, true)
	if err != nil { t.Fatal(err) }
	page, err := shop.Search(context.Background(), ShopQuery{})
	if err != nil { t.Fatal(err) }
	if got := page.Projects[0].Distribution; got != ShopDistributionWebsiteOnly {
		t.Fatalf("distribution = %q", got)
	}
}
```

- [ ] **Step 2: Run the tests and verify RED**

```powershell
rtk go test ./internal/scripting -run "TestShop(Categories|ProjectMapsDistribution)" -count=1
```

Expected: compile failure because `ShopCategory`, `Categories`, and `Distribution` do not exist.

- [ ] **Step 3: Implement the minimal bridge**

Add these types to `shop.go`:

```go
type ShopDistribution string

const (
	ShopDistributionUnknown     ShopDistribution = ""
	ShopDistributionDirect      ShopDistribution = "direct"
	ShopDistributionWebsiteOnly ShopDistribution = "website_only"
)

type ShopCategory struct {
	ID              string
	Name            string
	Slug            string
	LocalizationKey string
}
```

Add `Distribution ShopDistribution` to `ShopProject`. Refactor `call` into a shared
`callFunction(..., optional bool, ...)` implementation; when `optional` is true
and the global is not a function, close the Lua state and return `(nil, false,
nil)`. Keep existing `call` behavior unchanged for required functions. Add:

```go
func (s *LuaShop) Categories(ctx context.Context, kind string) ([]ShopCategory, error) {
	tbl, found, err := s.callOptional(ctx, "categories", map[string]lua.LValue{
		"kind": lua.LString(kind),
	})
	if err != nil || !found { return nil, err }
	items, _ := tbl.RawGetString("categories").(*lua.LTable)
	if items == nil { return nil, nil }
	out := make([]ShopCategory, 0, items.Len())
	items.ForEach(func(_, value lua.LValue) {
		if item, ok := value.(*lua.LTable); ok {
			out = append(out, ShopCategory{
				ID: strField(item, "id"), Name: strField(item, "name"),
				Slug: strField(item, "slug"),
				LocalizationKey: strField(item, "localization_key"),
			})
		}
	})
	return out, nil
}
```

In `readProject`, normalize only the supported values:

```go
distribution := ShopDistribution(strings.ToLower(strField(tbl, "distribution")))
if distribution != ShopDistributionDirect && distribution != ShopDistributionWebsiteOnly {
	distribution = ShopDistributionUnknown
}
```

Assign `Distribution: distribution` in the returned `ShopProject`.

- [ ] **Step 4: Run the bridge tests and verify GREEN**

```powershell
rtk go test ./internal/scripting -run "TestShop(Categories|ProjectMapsDistribution)" -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit the bridge checkpoint**

```powershell
rtk git add engine/internal/scripting/shop.go engine/internal/scripting/shop_test.go
rtk git commit -m feat:add-shop-category-bridge
```

---

### Task 3: Implement CurseForge distribution and cached categories

**Files:**
- Modify: `engine/internal/scripting/builtin_shops/curseforge.lua:27-310`
- Modify: `engine/internal/scripting/builtin_shops_test.go:25-273`

**Interfaces:**
- Consumes: optional Lua `categories(ctx)` and project `distribution` from Task 2.
- Produces: CurseForge category IDs/localization keys, `categoryIds` search filtering, and project distribution mapping.

- [ ] **Step 1: Write failing CurseForge HTTP fixture tests**

Add tests that serve `/v1/categories`, capture query parameters, and return class
and child rows. The assertions must be:

```go
if got.Get("gameId") != "432" || got.Get("classId") != "6" {
	t.Fatalf("category params: %v", got)
}
if len(categories) != 1 || categories[0].ID != "409" ||
	categories[0].LocalizationKey != "shop.category.curseforge.technology" {
	t.Fatalf("categories: %#v", categories)
}
```

Extend `TestCurseForgeSearchParams` with `Categories: []string{"409", "410"}`
and assert:

```go
if got.Get("categoryIds") != "409,410" {
	t.Fatalf("categoryIds = %q", got.Get("categoryIds"))
}
```

Add a table-driven project mapping test with cases `{false,
ShopDistributionWebsiteOnly}`, `{true, ShopDistributionDirect}`, and `{nil,
ShopDistributionUnknown}`. For each case, return a CurseForge mod response with
the corresponding `allowModDistribution`, call `Detail`, and compare
`detail.Project.Distribution`.

For caching, wire `host.SetHTTPCache(httpcache.New(t.TempDir(), 0))` in the test
helper, call `Categories` twice, and assert the `/v1/categories` handler count is
one while the script TTL is fresh.

- [ ] **Step 2: Run the CurseForge tests and verify RED**

```powershell
rtk go test ./internal/scripting -run "TestCurseForge(Categories|SearchParams|Distribution)" -count=1
```

Expected: failures because the category function, category query, and distribution field are absent.

- [ ] **Step 3: Implement the CurseForge Lua behavior**

Add:

```lua
local TTL_CATEGORIES = 86400

local function class_id(kind)
  return kind == "plugin" and CLASS_BUKKIT_PLUGINS or CLASS_MODS
end

local function distribution(m)
  if m.allowModDistribution == false then return "website_only" end
  if m.allowModDistribution == true then return "direct" end
  return ""
end
```

Set `distribution = distribution(m)` in `project_card`. Add:

```lua
function categories(ctx)
  api_key = ctx.config.api_key
  local class = class_id(ctx.kind or "mod")
  local body = get(API .. "/v1/categories?gameId=" .. GAME_MINECRAFT
    .. "&classId=" .. class, TTL_CATEGORIES)
  local out = {}
  for _, category in ipairs(body.data or {}) do
    if category.isClass ~= true and category.classId == class then
      out[#out + 1] = {
        id = tostring(category.id),
        name = category.name,
        slug = category.slug,
        localization_key = "shop.category.curseforge." .. (category.slug or tostring(category.id)),
      }
    end
  end
  return { categories = out }
end
```

Use `class_id` in `search_url` and append selected numeric IDs:

```lua
if #(ctx.categories or {}) > 0 then
  q[#q + 1] = "categoryIds=" .. urlencode(table.concat(ctx.categories, ","))
end
```

- [ ] **Step 4: Run the CurseForge tests and verify GREEN**

```powershell
rtk go test ./internal/scripting -run "TestCurseForge" -count=1
```

Expected: PASS, including the existing download-URL fallback and non-distributable tests.

- [ ] **Step 5: Commit the CurseForge checkpoint**

```powershell
rtk git add engine/internal/scripting/builtin_shops/curseforge.lua engine/internal/scripting/builtin_shops_test.go
rtk git commit -m feat:add-cached-curseforge-categories
```

---

### Task 4: Expose categories and distribution through gRPC

**Files:**
- Create: `engine/internal/grpc/shopservice_test.go`
- Modify: `engine/internal/grpc/shopservice.go:45-113,430-444`

**Interfaces:**
- Consumes: `LuaShop.Categories`, `ShopCategory`, and `ShopProject.Distribution`.
- Produces: `ShopService.GetCategories` and proto mapping helpers.

- [ ] **Step 1: Write failing gRPC mapping tests**

In `shopservice_test.go`, add table-driven tests for:

```go
func TestProjectToProtoMapsDistribution(t *testing.T) {
	tests := []struct {
		name string
		in scripting.ShopDistribution
		want mcmanagerv1.ShopDistribution
	}{
		{"unknown", scripting.ShopDistributionUnknown, mcmanagerv1.ShopDistribution_SHOP_DISTRIBUTION_UNKNOWN},
		{"direct", scripting.ShopDistributionDirect, mcmanagerv1.ShopDistribution_SHOP_DISTRIBUTION_DIRECT},
		{"website", scripting.ShopDistributionWebsiteOnly, mcmanagerv1.ShopDistribution_SHOP_DISTRIBUTION_WEBSITE_ONLY},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := projectToProto("shop", scripting.ShopProject{Distribution: tt.in})
			if got.Distribution != tt.want { t.Fatalf("distribution = %v", got.Distribution) }
		})
	}
}
```

Also test a `categoriesToProto` helper with ID, name, slug, and localization key.

- [ ] **Step 2: Run the gRPC tests and verify RED**

```powershell
rtk go test ./internal/grpc -run "Test(ProjectToProtoMapsDistribution|CategoriesToProto)" -count=1
```

Expected: compile failure because the mapping helper and field mapping are missing.

- [ ] **Step 3: Implement the RPC and mappings**

Add `GetCategories` after `List`:

```go
func (s *ShopService) GetCategories(ctx context.Context, req *mcmanagerv1.ShopCategoriesRequest) (*mcmanagerv1.ShopCategoryList, error) {
	sh, err := s.shop(req.ShopId)
	if err != nil { return nil, err }
	categories, err := sh.Categories(ctx, kindString(req.Kind))
	if err != nil { return nil, mapShopError(err) }
	return &mcmanagerv1.ShopCategoryList{Categories: categoriesToProto(categories)}, nil
}
```

Map distribution in `projectToProto` with an exhaustive helper and implement
`categoriesToProto` by copying all four category fields.

- [ ] **Step 4: Run the focused packages and verify GREEN**

```powershell
rtk go test ./internal/grpc ./internal/scripting -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit the transport checkpoint**

```powershell
rtk git add engine/internal/grpc/shopservice.go engine/internal/grpc/shopservice_test.go
rtk git commit -m feat:expose-shop-categories
```

---

### Task 5: Add a unit-tested C# presentation policy

**Files:**
- Create: `app/JustHostMC.Core/ShopPresentationPolicy.cs`
- Create: `app/JustHostMC.Core.Tests/ShopPresentationPolicyTests.cs`

**Interfaces:**
- Consumes: generated `ShopDistribution` and `ShopCategory`.
- Produces: `ShopPrimaryActionKind`, `ShopPrimaryAction`, `DeterminePrimaryAction`, and `ResolveCategoryLabel`.

- [ ] **Step 1: Write failing xUnit tests**

Create tests covering unknown/direct install, website-only valid URL, website-only
invalid URL, localized category, and fallback category:

```csharp
[Theory]
[InlineData(ShopDistribution.Unknown, "", ShopPrimaryActionKind.Install, true)]
[InlineData(ShopDistribution.Direct, "", ShopPrimaryActionKind.Install, true)]
[InlineData(ShopDistribution.WebsiteOnly, "https://example.test/mod", ShopPrimaryActionKind.Website, true)]
[InlineData(ShopDistribution.WebsiteOnly, "", ShopPrimaryActionKind.Website, false)]
public void DeterminePrimaryActionMapsPolicy(ShopDistribution distribution,
                                              string website,
                                              ShopPrimaryActionKind kind,
                                              bool enabled) {
    var result = ShopPresentationPolicy.DeterminePrimaryAction(distribution, website);
    Assert.Equal(kind, result.Kind);
    Assert.Equal(enabled, result.IsEnabled);
}

[Fact]
public void ResolveCategoryLabelFallsBackWhenKeyIsMissing() {
    var category = new ShopCategory { Name = "Technology", LocalizationKey = "missing.key" };
    Assert.Equal("Technology", ShopPresentationPolicy.ResolveCategoryLabel(category, key => key));
}
```

- [ ] **Step 2: Run the tests and verify RED**

```powershell
rtk dotnet test app/JustHostMC.Core.Tests/JustHostMC.Core.Tests.csproj -p:SkipEngineBuild=true -p:SkipProtobufGenerate=true --filter ShopPresentationPolicy
```

Expected: compile failure because the policy types do not exist.

- [ ] **Step 3: Implement the pure policy**

```csharp
using McManager.Grpc;

namespace JustHostMC.Core;

public enum ShopPrimaryActionKind { Install, Website }

public readonly record struct ShopPrimaryAction(ShopPrimaryActionKind Kind,
                                                 bool IsEnabled);

public static class ShopPresentationPolicy {
    public static ShopPrimaryAction DeterminePrimaryAction(
        ShopDistribution distribution, string websiteUrl) {
        if (distribution != ShopDistribution.WebsiteOnly)
            return new(ShopPrimaryActionKind.Install, true);
        var valid = Uri.TryCreate(websiteUrl, UriKind.Absolute, out var uri) &&
                    (uri.Scheme == Uri.UriSchemeHttps || uri.Scheme == Uri.UriSchemeHttp);
        return new(ShopPrimaryActionKind.Website, valid);
    }

    public static string ResolveCategoryLabel(ShopCategory category,
                                               Func<string, string> resolve) {
        if (category.LocalizationKey.Length == 0) return category.Name;
        var localized = resolve(category.LocalizationKey);
        return localized == category.LocalizationKey ? category.Name : localized;
    }
}
```

- [ ] **Step 4: Run the policy tests and verify GREEN**

```powershell
rtk dotnet test app/JustHostMC.Core.Tests/JustHostMC.Core.Tests.csproj -p:SkipEngineBuild=true -p:SkipProtobufGenerate=true --filter ShopPresentationPolicy
```

Expected: PASS.

- [ ] **Step 5: Commit the presentation checkpoint**

```powershell
rtk git add app/JustHostMC.Core/ShopPresentationPolicy.cs app/JustHostMC.Core.Tests/ShopPresentationPolicyTests.cs
rtk git commit -m feat:add-shop-presentation-policy
```

---

### Task 6: Wire dynamic categories and website-only actions into WinUI

**Files:**
- Modify: `app/JustHostMC.App/ViewModels/ShopViewModel.cs:15-126`
- Modify: `app/JustHostMC.App/ViewModels/ShopDetailViewModel.cs:14-267`
- Modify: `app/JustHostMC.App/Models/ShopModels.cs:87-122`
- Modify: `app/JustHostMC.App/Views/ShopDetailPage.xaml:126-355`
- Modify: `app/JustHostMC.App/Views/ShopDetailPage.xaml.cs:89-131`
- Modify: `app/JustHostMC.App/Strings/en-US/Resources.resw:620-655`
- Modify: `app/JustHostMC.App/Strings/zh-TW/Resources.resw:605-640`

**Interfaces:**
- Consumes: `GetCategories`, `ShopPresentationPolicy`, runtime `ShopInfo.name`, and distribution from project results/detail.
- Produces: dynamic localized filters and a single safe primary action across latest/version buttons.

- [ ] **Step 1: Load source-provided categories safely**

Add `_categoryGeneration`. In `OnSelectedShopChanged`, retain the existing static
Modrinth list, otherwise call `LoadCategoriesAsync(value, generation)`. The async
method sends `ShopCategoriesRequest { ShopId = shop.Id, Kind = Context.Kind }`,
uses `ShopPresentationPolicy.ResolveCategoryLabel(category, _localizer.Get)`, and
updates the collection only if both generation and selected shop still match.
On failure, leave the collection empty; do not block home/search.

- [ ] **Step 2: Add observable per-version action state**

Change `ShopVersionItem` to a partial `ObservableObject` and add:

```csharp
[ObservableProperty]
public partial string ActionLabel { get; set; } = "";

[ObservableProperty]
public partial bool ActionEnabled { get; set; }
```

- [ ] **Step 3: Derive the detail primary action and dynamic label**

In `ShopDetailViewModel`, add:

```csharp
public ShopPrimaryAction PrimaryAction =>
    ShopPresentationPolicy.DeterminePrimaryAction(Card.Project.Distribution,
                                                  WebsiteUrl);
public bool IsWebsiteAction => PrimaryAction.Kind == ShopPrimaryActionKind.Website;
public string PrimaryActionLabel => IsWebsiteAction
    ? _localizer.Get("Shop_GetOnSource", ("source", SourceName))
    : _localizer.Get("Shop_InstallAction");
```

Create `RefreshPrimaryAction()` that raises these properties, recalculates
`CanInstallLatest`, and assigns `ActionLabel`/`ActionEnabled` on every version.
Call it after detail changes `Card`/`WebsiteUrl`, after versions are populated,
and whenever `IsInstalling` changes. `CanInstallLatest` requires a latest release,
`PrimaryAction.IsEnabled`, and `!IsInstalling`.

- [ ] **Step 4: Bind labels and short-circuit website actions**

Set the two latest buttons' `Content` to
`{x:Bind ViewModel.PrimaryActionLabel, Mode=OneWay}`. In the version template set:

```xml
Content="{x:Bind ActionLabel, Mode=OneWay}"
IsEnabled="{x:Bind ActionEnabled, Mode=OneWay}"
```

At the start of `OnInstallClick`, after reading the version tag, add:

```csharp
if (ViewModel.IsWebsiteAction) {
    await OpenWebsiteAsync();
    return;
}
```

Extract the URI validation/launch code shared by `OnOpenWebsite` into
`OpenWebsiteAsync`. This return must occur before dependency calculation and the
dialog.

- [ ] **Step 5: Add dynamic action and category resources**

Add:

```xml
<data name="Shop_InstallAction" xml:space="preserve"><value>Install</value></data>
<data name="Shop_GetOnSource" xml:space="preserve"><value>Get on {source}</value></data>
```

and Traditional Chinese:

```xml
<data name="Shop_InstallAction" xml:space="preserve"><value>安裝</value></data>
<data name="Shop_GetOnSource" xml:space="preserve"><value>前往 {source} 取得</value></data>
```

Add en-US and zh-TW entries under
`shop_category_curseforge_<slug-with-hyphens-preserved>` using these exact
slug/value mappings; newly introduced upstream slugs use the runtime fallback:

| Slug | en-US | zh-TW |
|---|---|---|
| `adventure-and-rpg` | Adventure and RPG | 冒險與角色扮演 |
| `api-and-library` | API and Library | API 與函式庫 |
| `armor-weapons-tools` | Armor, Weapons, and Tools | 護甲、武器與工具 |
| `cosmetic` | Cosmetic | 外觀 |
| `education` | Education | 教育 |
| `food` | Food | 食物 |
| `game-mechanics` | Game Mechanics | 遊戲機制 |
| `magic` | Magic | 魔法 |
| `map-and-information` | Map and Information | 地圖與資訊 |
| `mc-miscellaneous` | Minecraft Miscellaneous | Minecraft 雜項 |
| `mobs` | Mobs | 生物 |
| `optimization` | Optimization | 效能最佳化 |
| `redstone` | Redstone | 紅石 |
| `server-utility` | Server Utility | 伺服器工具 |
| `storage` | Storage | 儲存 |
| `technology` | Technology | 科技 |
| `transport` | Transport | 運輸 |
| `world-gen` | World Generation | 世界生成 |
| `admin-tools` | Admin Tools | 管理工具 |
| `anti-griefing-tools` | Anti-Griefing Tools | 防破壞工具 |
| `chat-related` | Chat Related | 聊天 |
| `developer-tools` | Developer Tools | 開發工具 |
| `economy` | Economy | 經濟 |
| `fixes` | Fixes | 修正 |
| `fun` | Fun | 娛樂 |
| `general` | General | 一般 |
| `informational` | Informational | 資訊 |
| `mechanics` | Mechanics | 機制 |
| `miscellaneous` | Miscellaneous | 雜項 |
| `role-playing` | Role Playing | 角色扮演 |
| `teleportation` | Teleportation | 傳送 |
| `world-editing-and-management` | World Editing and Management | 世界編輯與管理 |
| `world-generators` | World Generators | 世界生成器 |

- [ ] **Step 6: Build the WinUI project**

```powershell
rtk dotnet build app/JustHostMC.App/JustHostMC.App.csproj -p:Platform=x64 -p:SkipEngineBuild=true -p:SkipProtobufGenerate=true
```

Expected: exit 0 with no MVVM Toolkit analyzer warnings or XAML compiler errors.

- [ ] **Step 7: Commit the UI checkpoint**

```powershell
rtk git add app/JustHostMC.App app/JustHostMC.Core
rtk git commit -m feat:add-source-driven-shop-actions
```

---

### Task 7: Document the extended script contract and verify end to end

**Files:**
- Modify: `docs/scripting.md:522-590`
- Verify: all files changed by Tasks 1-6

**Interfaces:**
- Consumes: the completed contract and behavior.
- Produces: author-facing documentation and final verification evidence.

- [ ] **Step 1: Update shop-script documentation**

Document optional `categories(ctx)`, its `{id,name,slug,localization_key}` result,
project `distribution`, the three-state semantics, the 24-hour category cache
example, and the rule that missing optional categories returns an empty list.

- [ ] **Step 2: Format and inspect the complete diff**

```powershell
rtk gofmt -w engine/internal/scripting/shop.go engine/internal/scripting/shop_test.go engine/internal/scripting/builtin_shops_test.go engine/internal/grpc/shopservice.go engine/internal/grpc/shopservice_test.go
rtk git diff --check
rtk git diff --stat
```

Expected: formatting succeeds; `git diff --check` reports no whitespace errors.

- [ ] **Step 3: Run all Go tests**

```powershell
rtk go test ./...
```

Expected: PASS; gated real-server tests skip unless `JHMC_INTEGRATION=1`.

- [ ] **Step 4: Build the bundled engine**

```powershell
rtk go build -trimpath -buildvcs=false -mod=readonly -ldflags=-s -o ../build/engine.exe ./cmd/engine
```

Run from `engine/`. Expected: exit 0 and `build/engine.exe` exists.

- [ ] **Step 5: Run .NET tests**

```powershell
rtk dotnet test app/JustHostMC.Core.Tests/JustHostMC.Core.Tests.csproj -p:SkipEngineBuild=true -p:SkipProtobufGenerate=true
```

Expected: PASS, including `ShopPresentationPolicyTests` and engine integration tests.

- [ ] **Step 6: Build the full solution**

```powershell
rtk dotnet build JustHostMC.sln -p:Platform=x64 -p:SkipEngineBuild=true -p:SkipProtobufGenerate=true
```

Expected: exit 0 with no warnings introduced by this change.

- [ ] **Step 7: Verify requirements against the diff**

Confirm from the final diff that:

- no UI comparison to the literal shop ID `curseforge` controls the website action;
- website-only clicks return before dependency/install flow;
- category lookup uses `jhmc.http_cache` and TTL `86400`;
- CurseForge search emits `classId` and `categoryIds` separately;
- unknown localization keys use the upstream name;
- `resolve_file` still blocks missing download URLs;
- generated Go stubs and `build/engine.exe` are untracked/ignored.

- [ ] **Step 8: Commit documentation and any verification fixes**

```powershell
rtk git add docs/scripting.md
rtk git commit -m docs:extend-shop-script-contract
rtk git status --short
```

Expected: only intentionally ignored generated/build artifacts remain absent from status.
