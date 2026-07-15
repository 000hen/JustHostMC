# Server Change Stream Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace three-second frontend polling with a race-free gRPC stream that emits one changed server per event while retaining `List` for initial and reconnect reconciliation.

**Architecture:** Wrap the production server store with an observable decorator backed by a bounded fan-out broker. `ServerService.WatchChanges` registers a subscriber, sends `ready`, and then maps each store mutation to one protobuf event. A testable C# synchronizer performs `ready -> List -> events` with reconnect backoff, while `MainViewModel` applies each event on the WinUI dispatcher.

**Tech Stack:** Protocol Buffers/buf, Go 1.24+ with gRPC-Go, C#/.NET 9 with Grpc.Net.Client, WinUI 3, xUnit.

## Global Constraints

- A stream subscription receives no existing server snapshots; initial state comes from `ServerService.List`.
- Every change message describes exactly one server: full `Server` for upsert or `ServerId` for deletion.
- Register the subscriber before sending `ready`, so events produced during `List` are queued and applied afterward.
- Publish only after successful persistence, never block persistence on a slow subscriber, and reconnect through a fresh `List` after stream loss.
- Marshal every frontend stream update through `DispatcherQueue.TryEnqueue` via the existing `RunOnUI` pattern.
- Regenerate Go protobuf stubs after editing `proto/mcmanager/v1/mcmanager.proto`; C# stubs regenerate during build.
- Do not commit `engine/gen/` or `build/engine.exe`.
- Prefix every shell command with `rtk`.

---

### Task 1: Add the server-change protobuf contract

**Files:**

- Modify: `proto/mcmanager/v1/mcmanager.proto`
- Generated only: `engine/gen/mcmanager/v1/*`

**Interfaces:**

- Produces: `ServerChangeEvent` with `ready`, `upsert`, and `deleted` oneof variants.
- Produces: `ServerService.WatchChanges(Empty) returns (stream ServerChangeEvent)`.

- [ ] **Step 1: Add a compile-time contract test before changing the proto**

Create `engine/internal/grpc/serverchanges_contract_test.go` with a test that references the desired generated API:

```go
package grpcsvc

import (
    "testing"

    mcmanagerv1 "github.com/000hen/justhostmc/engine/gen/mcmanager/v1"
)

func TestServerChangeContractHasSingleItemVariants(t *testing.T) {
    tests := []struct {
        name  string
        event *mcmanagerv1.ServerChangeEvent
    }{
        {"ready", &mcmanagerv1.ServerChangeEvent{Change: &mcmanagerv1.ServerChangeEvent_Ready{Ready: &mcmanagerv1.Empty{}}}},
        {"upsert", &mcmanagerv1.ServerChangeEvent{Change: &mcmanagerv1.ServerChangeEvent_Upsert{Upsert: &mcmanagerv1.Server{Id: "one"}}}},
        {"deleted", &mcmanagerv1.ServerChangeEvent{Change: &mcmanagerv1.ServerChangeEvent_Deleted{Deleted: &mcmanagerv1.ServerId{Id: "one"}}}},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if tt.event.GetChange() == nil {
                t.Fatal("change variant is nil")
            }
        })
    }
}
```

- [ ] **Step 2: Run the contract test and verify RED**

Run from `engine/`:

```text
rtk go test ./internal/grpc -run TestServerChangeContractHasSingleItemVariants
```

Expected: compilation fails because `ServerChangeEvent` and its oneof wrappers do not exist.

- [ ] **Step 3: Add the message and streaming RPC**

Add after `ServerList`:

```proto
message ServerChangeEvent {
  oneof change {
    Empty ready = 1;
    Server upsert = 2;
    ServerId deleted = 3;
  }
}
```

Add to `ServerService` immediately after `List`:

```proto
rpc WatchChanges(Empty) returns (stream ServerChangeEvent);
```

- [ ] **Step 4: Regenerate Go stubs and verify GREEN**

Run from `proto/`:

```text
rtk buf generate
```

Then from `engine/`:

```text
rtk go test ./internal/grpc -run TestServerChangeContractHasSingleItemVariants
```

Expected: PASS.

- [ ] **Step 5: Commit the contract**

```text
rtk git add proto/mcmanager/v1/mcmanager.proto engine/internal/grpc/serverchanges_contract_test.go
rtk git commit -m feat-server-change-contract
```

---

### Task 2: Build the observable store and bounded broker

**Files:**

- Create: `engine/internal/store/observable.go`
- Create: `engine/internal/store/observable_test.go`

**Interfaces:**

- Consumes: existing `store.Store`.
- Produces: `ChangeKind`, `Change`, `Subscription`, `ChangeSource`, `Observable`, and `NewObservable(Store, int)`.
- `Observable` implements `Store`; `Subscribe()` returns a bounded event channel plus idempotent cancellation.

- [ ] **Step 1: Write failing observable-store tests**

Cover these externally visible behaviors in table-driven subtests:

```go
func TestObservablePublishesSuccessfulMutations(t *testing.T) {
    base := NewMemory()
    observable := NewObservable(base, 4)
    subscription := observable.Subscribe()
    defer subscription.Cancel()

    rec := &Server{ID: "one", Name: "First", SortOrder: 2}
    if err := observable.Put(rec); err != nil { t.Fatal(err) }
    rec.Name = "mutated after put"

    upsert := <-subscription.Events
    if upsert.Kind != ChangeUpsert || upsert.Server.Name != "First" {
        t.Fatalf("upsert = %#v", upsert)
    }

    if err := observable.Delete("one"); err != nil { t.Fatal(err) }
    deleted := <-subscription.Events
    if deleted.Kind != ChangeDeleted || deleted.ServerID != "one" {
        t.Fatalf("deleted = %#v", deleted)
    }
}
```

Add `TestObservableDoesNotPublishFailedMutation` using a `failingStore` whose `Put` and `Delete` return sentinel errors. Add `TestObservableDropsOnlySlowSubscriber` with capacity one: fill one subscriber, publish again, assert its channel closes, and assert a concurrently drained subscriber still receives both events. Add `TestSubscriptionCancelIsIdempotent`.

- [ ] **Step 2: Run tests and verify RED**

From `engine/`:

```text
rtk go test ./internal/store -run Observable
```

Expected: compilation fails because `NewObservable` and change types are undefined.

- [ ] **Step 3: Implement the minimal observable store**

Use these public shapes:

```go
type ChangeKind uint8

const (
    ChangeUpsert ChangeKind = iota + 1
    ChangeDeleted
)

type Change struct {
    Kind     ChangeKind
    Server   *Server
    ServerID string
}

type Subscription struct {
    Events <-chan Change
    cancel func()
}

func (s Subscription) Cancel() { s.cancel() }

type ChangeSource interface {
    Subscribe() Subscription
}

type Observable struct {
    Store
    broker *changeBroker
}

func NewObservable(base Store, capacity int) *Observable
func (s *Observable) Put(server *Server) error
func (s *Observable) Delete(id string) error
func (s *Observable) Subscribe() Subscription
```

The broker holds `map[uint64]chan Change` under one mutex. `publish` performs a non-blocking send while holding the mutex; on a full channel it removes and closes only that channel. Clone `Server` and its `LaunchArgs` before publishing so later mutation cannot alter an event snapshot. Invoke `publish` only after the wrapped operation returns nil.

- [ ] **Step 4: Run tests and verify GREEN**

```text
rtk go test ./internal/store -run Observable
rtk go test ./internal/store
```

Expected: PASS with no race or panic.

- [ ] **Step 5: Commit the observable store**

```text
rtk git add engine/internal/store/observable.go engine/internal/store/observable_test.go
rtk git commit -m feat-observable-server-store
```

---

### Task 3: Serve change events and wire every production mutation through the observable store

**Files:**

- Create: `engine/internal/grpc/serverchanges_test.go`
- Modify: `engine/internal/grpc/serverservice.go`
- Modify: `engine/cmd/engine/main.go`

**Interfaces:**

- Consumes: `store.ChangeSource.Subscribe()`.
- Produces: `ServerService.WatchChanges(*Empty, grpc.ServerStreamingServer[ServerChangeEvent]) error`.
- Production invariant: every service receives the same observable `registry` wrapper; only the underlying SQLite handle is closed.

- [ ] **Step 1: Write failing stream tests**

Use a generated gRPC test server over `bufconn` or the repository's existing server registration helper. The main test sequence is:

```go
observable := store.NewObservable(store.NewMemory(), 4)
service := NewServerService(ServerServiceConfig{Store: observable, Changes: observable})
// Start WatchChanges in a goroutine and receive the first message.
// Assert first.GetReady() != nil and first.GetUpsert() == nil.
// Put one record through observable and assert the next message is one upsert.
// Delete it and assert the next message contains only Deleted.Id.
```

Add a cancellation test that cancels the client context and expects the server handler to exit. Add an overflow test with capacity one that publishes without draining and expects `codes.ResourceExhausted` after the already-sent `ready` marker.

- [ ] **Step 2: Run stream tests and verify RED**

From `engine/`:

```text
rtk go test ./internal/grpc -run WatchChanges
```

Expected: compilation fails because `ServerServiceConfig.Changes` and `WatchChanges` do not exist.

- [ ] **Step 3: Implement `WatchChanges`**

Add to `ServerServiceConfig`:

```go
Changes store.ChangeSource
```

Implement the handler with this flow:

```go
func (s *ServerService) WatchChanges(_ *mcmanagerv1.Empty, stream grpc.ServerStreamingServer[mcmanagerv1.ServerChangeEvent]) error {
    if s.cfg.Changes == nil {
        return status.Error(codes.Unavailable, "server change stream unavailable")
    }
    subscription := s.cfg.Changes.Subscribe()
    defer subscription.Cancel()

    if err := stream.Send(&mcmanagerv1.ServerChangeEvent{
        Change: &mcmanagerv1.ServerChangeEvent_Ready{Ready: &mcmanagerv1.Empty{}},
    }); err != nil {
        return err
    }

    for {
        select {
        case <-stream.Context().Done():
            return stream.Context().Err()
        case change, ok := <-subscription.Events:
            if !ok {
                return status.Error(codes.ResourceExhausted, "server change subscriber fell behind")
            }
            event := &mcmanagerv1.ServerChangeEvent{}
            switch change.Kind {
            case store.ChangeUpsert:
                event.Change = &mcmanagerv1.ServerChangeEvent_Upsert{Upsert: change.Server.Proto()}
            case store.ChangeDeleted:
                event.Change = &mcmanagerv1.ServerChangeEvent_Deleted{Deleted: &mcmanagerv1.ServerId{Id: change.ServerID}}
            default:
                continue
            }
            if err := stream.Send(event); err != nil { return err }
        }
    }
}
```

- [ ] **Step 4: Wrap and wire the production store**

In `main.go`, retain `sqliteRegistry` for `Close`, then create:

```go
sqliteRegistry, err := store.OpenSQLite(filepath.Join(paths.Base, "registry.db"))
if err != nil { log.Fatalf("[FATAL] open registry: %v", err) }
defer sqliteRegistry.Close()
registry := store.NewObservable(sqliteRegistry, 64)
```

Pass `registry` everywhere currently receiving the SQLite store, and set `Changes: registry` in `ServerServiceConfig`. This includes backup, players, automation query adapter, mod, config, shop, and server services.

- [ ] **Step 5: Run focused and full Go verification**

```text
rtk go test ./internal/grpc -run WatchChanges
rtk go test ./...
```

Expected: PASS. Existing lifecycle tests continue to pass because `Store` remains unchanged.

- [ ] **Step 6: Commit the daemon stream**

```text
rtk git add engine/internal/grpc/serverservice.go engine/internal/grpc/serverchanges_test.go engine/cmd/engine/main.go
rtk git commit -m feat-stream-server-changes
```

---

### Task 4: Add testable C# synchronization and single-item state reduction

**Files:**

- Create: `app/JustHostMC.Core/ServerChangeSource.cs`
- Create: `app/JustHostMC.Core/ServerChangeSynchronizer.cs`
- Create: `app/JustHostMC.Core/ServerListState.cs`
- Create: `app/JustHostMC.Core.Tests/ServerChangeSynchronizerTests.cs`
- Create: `app/JustHostMC.Core.Tests/ServerListStateTests.cs`
- Create: `app/JustHostMC.Core.Tests/ServerChangesIntegrationTests.cs`

**Interfaces:**

- Produces: `IServerChangeSource.WatchAsync` and `ListAsync`.
- Produces: `GrpcServerChangeSource` over generated `ServerServiceClient`.
- Produces: `ServerChangeSynchronizer.RunAsync` with injectable retry delay.
- Produces: `ServerListState.Reconcile`, `Apply`, and ordered `Servers` snapshots.

- [ ] **Step 1: Write failing state-reducer tests**

Test this public behavior:

```csharp
[Fact]
public void Apply_UpsertsDeletesAndSortsSingleServers() {
    var state = new ServerListState();
    state.Reconcile([
        new Server { Id = "b", Name = "B", SortOrder = 1 },
        new Server { Id = "a", Name = "A", SortOrder = 0 },
    ]);

    state.Apply(new ServerChangeEvent {
        Upsert = new Server { Id = "b", Name = "B2", SortOrder = -1 },
    });
    Assert.Equal(["b", "a"], state.Servers.Select(s => s.Id));
    Assert.Equal("B2", state.Servers[0].Name);

    state.Apply(new ServerChangeEvent {
        Deleted = new ServerId { Id = "a" },
    });
    Assert.Single(state.Servers);
}
```

Also test unknown deletion is a no-op and `Reconcile` removes stale entries.

- [ ] **Step 2: Write failing synchronizer tests**

Implement a fake `IServerChangeSource` using channels. Verify:

- `RunAsync` does not call `ListAsync` before `ready`.
- After `ready`, it calls `ListAsync`, invokes reconcile once, then applies queued events in order.
- A completed/failed stream invokes the injected retry delay and starts a fresh watch plus list.
- Cancellation exits without retry.

Use constructor injection `Func<TimeSpan, CancellationToken, Task> delay` so tests complete without sleeping.

- [ ] **Step 3: Run C# tests and verify RED**

From the repository root:

```text
rtk test dotnet test app/JustHostMC.Core.Tests/JustHostMC.Core.Tests.csproj -p:SkipEngineBuild=true -p:SkipProtobufGenerate=true --filter "FullyQualifiedName~ServerChange"
```

Expected: compilation fails because the source, synchronizer, and state classes do not exist.

- [ ] **Step 4: Implement `ServerListState`**

Store cloned protobuf messages in a private list. `Reconcile` replaces the list with clones sorted by `SortOrder`, then `Name`, then `Id`. `Apply` replaces or inserts exactly one `Upsert`, removes exactly one `Deleted.Id`, ignores `Ready`/unknown variants, and re-sorts after upsert.

- [ ] **Step 5: Implement the source and synchronizer**

Use these signatures:

```csharp
public interface IServerChangeSource {
    IAsyncEnumerable<ServerChangeEvent> WatchAsync(CancellationToken cancellationToken);
    Task<IReadOnlyList<Server>> ListAsync(CancellationToken cancellationToken);
}

public sealed class GrpcServerChangeSource(
    ServerService.ServerServiceClient client) : IServerChangeSource;

public sealed class ServerChangeSynchronizer(
    Func<TimeSpan, CancellationToken, Task>? delay = null) {
    public Task RunAsync(
        IServerChangeSource source,
        Action<IReadOnlyList<Server>> reconcile,
        Action<ServerChangeEvent> apply,
        CancellationToken cancellationToken);
}
```

`GrpcServerChangeSource.WatchAsync` owns/disposes the `AsyncServerStreamingCall` and yields `ReadAllAsync(cancellationToken)`. `ListAsync` uses a ten-second deadline. `RunAsync` requires the first event to be `Ready`, lists before reading the next event, retries all non-cancellation failures with delays of 250 ms, 500 ms, 1 s, 2 s, and then 5 s maximum, and resets the backoff only after successful readiness/list synchronization.

- [ ] **Step 6: Add an engine integration test for the wire handshake**

Launch the existing `EngineFixture`, create `DaemonClient`, open `WatchChanges`, and assert the first message has `Ready` and no `Upsert`/`Deleted`. Call `List` and use a short cancellation token to assert no pre-existing server is replayed through the stream.

- [ ] **Step 7: Run tests and verify GREEN**

```text
rtk test dotnet test app/JustHostMC.Core.Tests/JustHostMC.Core.Tests.csproj --filter "FullyQualifiedName~ServerChange"
```

Expected: PASS after Engine.targets regenerates C# stubs and rebuilds `engine.exe`.

- [ ] **Step 8: Commit C# synchronization primitives**

```text
rtk git add app/JustHostMC.Core app/JustHostMC.Core.Tests
rtk git commit -m feat-synchronize-server-changes
```

---

### Task 5: Replace MainViewModel polling with the change synchronizer

**Files:**

- Modify: `app/JustHostMC.App/ViewModels/MainViewModel.cs`
- Modify: `app/JustHostMC.App/MainWindow.xaml.cs`

**Interfaces:**

- Consumes: `GrpcServerChangeSource`, `ServerChangeSynchronizer`, and `ServerListState`.
- Produces: one long-lived `MainViewModel` stream loop and `Dispose()` cancellation.

- [ ] **Step 1: Establish the build-level RED check**

Before editing `MainViewModel`, run:

```text
rtk dotnet build app/JustHostMC.App/JustHostMC.App.csproj -p:Platform=x64 -p:SkipEngineBuild=true
```

Expected: PASS. This records the clean baseline; behavior remains red through the Task 4 synchronizer tests until the view model consumes the new API.

- [ ] **Step 2: Replace timer fields with stream state**

Make `MainViewModel` implement `IDisposable`. Replace `_refreshTimer` with:

```csharp
private readonly CancellationTokenSource _serverChangesCts = new();
private readonly ServerChangeSynchronizer _serverChanges = new();
private readonly ServerListState _serverState = new();
private Task? _serverChangesTask;
```

- [ ] **Step 3: Start the stream after health succeeds**

In `ConnectAsync`, replace `await RefreshAsync(); StartAutoRefresh();` with a single guarded task:

```csharp
_serverChangesTask ??= RunServerChangesAsync(daemon);
```

Add:

```csharp
private Task RunServerChangesAsync(JustHostMC.Core.DaemonClient daemon) =>
    _serverChanges.RunAsync(
        new GrpcServerChangeSource(daemon.Servers),
        servers => RunOnUI(() => MergeServers(servers)),
        change => RunOnUI(() => ApplyServerChange(change)),
        _serverChangesCts.Token);
```

Keep `RefreshAsync` only for the explicit `[RelayCommand] Refresh` action.

- [ ] **Step 4: Apply one event without replacing unaffected items**

At the start of `MergeServers`, call `_serverState.Reconcile(list)`. Extract the existing tracker/item logic into `UpsertServer(Server proto)`. Add `ApplyServerChange`:

```csharp
private void ApplyServerChange(ServerChangeEvent change) {
    _serverState.Apply(change);
    switch (change.ChangeCase) {
    case ServerChangeEvent.ChangeOneofCase.Upsert:
        UpsertServer(change.Upsert);
        ReorderServers(_serverState.Servers.Select(s => s.Id));
        break;
    case ServerChangeEvent.ChangeOneofCase.Deleted:
        RemoveServer(change.Deleted.Id);
        break;
    }
    OnServerStatsChanged();
}
```

`RemoveServer` removes the matching `ServerItem` and the same object from `NavigationItems`; unknown IDs return without change. `ReorderServers` moves existing objects and keeps the Home entry offset of one in `NavigationItems`.

- [ ] **Step 5: Remove automatic polling refreshes**

Delete `StartAutoRefresh`. Remove post-success `RefreshAsync` calls from install, update, start, stop, and delete. Preserve rollback on a failed optimistic update, but do not list on failure. The successful `Update` return can be applied immediately through the normal event; the store event is authoritative.

- [ ] **Step 6: Cancel the stream on window close**

Implement:

```csharp
public void Dispose() {
    _serverChangesCts.Cancel();
    _serverChangesCts.Dispose();
}
```

Call `Shell.Main.Dispose()` at the start of `MainWindow.OnClosed` before detaching collection handlers.

- [ ] **Step 7: Build and run C# tests**

```text
rtk test dotnet test app/JustHostMC.Core.Tests/JustHostMC.Core.Tests.csproj
rtk dotnet build app/JustHostMC.App/JustHostMC.App.csproj -p:Platform=x64
```

Expected: all tests PASS and the x64 WinUI build exits 0 with no MVVM Toolkit analyzer warnings.

- [ ] **Step 8: Commit the frontend migration**

```text
rtk git add app/JustHostMC.App/ViewModels/MainViewModel.cs app/JustHostMC.App/MainWindow.xaml.cs
rtk git commit -m feat-replace-server-polling
```

---

### Task 6: Final verification and documentation consistency

**Files:**

- Modify if needed: `docs/superpowers/specs/2026-07-12-server-change-stream-design.md`
- Modify if needed: `CLAUDE.md`

**Interfaces:**

- Verifies the complete proto -> daemon -> named-pipe gRPC -> frontend path.

- [ ] **Step 1: Format changed code**

From `engine/`:

```text
rtk proxy gofmt -w internal/store/observable.go internal/store/observable_test.go internal/grpc/serverchanges_contract_test.go internal/grpc/serverchanges_test.go internal/grpc/serverservice.go cmd/engine/main.go
```

From the repository root:

```text
rtk proxy dotnet format JustHostMC.sln --no-restore
```

- [ ] **Step 2: Run the full Go suite with race detection on changed concurrent packages**

From `engine/`:

```text
rtk go test -race ./internal/store ./internal/grpc
rtk go test ./...
```

Expected: PASS.

- [ ] **Step 3: Run full C# tests and build**

From the repository root:

```text
rtk test dotnet test app/JustHostMC.Core.Tests/JustHostMC.Core.Tests.csproj
rtk dotnet build JustHostMC.sln -p:Platform=x64
```

Expected: all tests PASS and solution build exits 0.

- [ ] **Step 4: Confirm polling removal and event cardinality**

```text
rtk grep "StartAutoRefresh|FromSeconds(3)|await RefreshAsync" app/JustHostMC.App/ViewModels/MainViewModel.cs
rtk grep "WatchChanges|ServerChangeEvent" proto/mcmanager/v1/mcmanager.proto engine/internal/grpc app/JustHostMC.Core app/JustHostMC.App
rtk git diff --check
rtk git status --short
```

Expected: no timer match; remaining `RefreshAsync` references belong only to the explicit refresh command; every change-stream send constructs exactly one oneof variant; `git diff --check` is clean.

- [ ] **Step 5: Commit any final documentation correction**

Only if verification required a documentation change:

```text
rtk git add docs/superpowers/specs/2026-07-12-server-change-stream-design.md CLAUDE.md
rtk git commit -m docs-server-change-stream
```
