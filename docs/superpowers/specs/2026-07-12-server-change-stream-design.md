# Server Change Stream Design

## Goal

Replace the frontend's three-second server-list polling with a daemon-driven
gRPC stream. The initial state still comes from `ServerService.List`; after the
client subscribes, every stream message describes exactly one server change.

## Scope

This change covers server registry mutations visible in the main server list:
creation, edits, ordering, lifecycle status transitions, deletion, startup
reconciliation, and unexpected process exit. It does not replace unrelated
metrics, console, player, script, or install-progress streams.

## Protocol

Add `WatchChanges(Empty) returns (stream ServerChangeEvent)` to
`ServerService`.

`ServerChangeEvent` uses a protobuf `oneof` with three variants:

- `ready`: an empty marker sent once after the daemon has registered the
  subscriber.
- `upsert`: one complete `Server` snapshot for a created or changed server.
- `deleted`: one `ServerId` identifying a removed server.

The stream never sends `ServerList` and never replays existing servers.
Existing servers are obtained only through `List`.

## Daemon Architecture

Introduce an observable `store.Store` decorator that owns a server-change
broker and delegates persistence to the existing SQLite or memory store.
After a successful `Put`, it publishes an immutable `upsert` snapshot. After a
successful `Delete`, it publishes `deleted`. Failed persistence publishes
nothing.

The decorator is the single notification boundary. This ensures changes from
`ServerService`, startup reconciliation, process-exit watchers, and other
services using the shared store follow the same path without duplicating
notification calls across RPC implementations.

Each subscriber has a bounded buffered channel. Publishing never blocks a
server operation. If a subscriber's channel is full, the broker terminates that
subscription; the stream returns a retryable gRPC error and the client
reconnects and reconciles with `List`. Unsubscribing is idempotent and happens
when the RPC context is cancelled or the send loop exits.

Events are ordered in the same order that successful decorator mutations are
published. They are process-local and are not persisted or replayed after a
disconnect.

## Frontend Startup and Reconnection

`MainViewModel` replaces its dispatcher timer with one long-running change
stream loop:

1. Open `WatchChanges` and read until `ready` is received.
2. Call `List` for the authoritative initial snapshot.
3. Apply that list on the UI thread.
4. Continue reading and apply subsequent single-server events in stream order.

The daemon registers the subscriber before sending `ready`. Consequently,
changes made while `List` is in flight are queued behind `ready` and are applied
after the possibly older list snapshot, closing the list/subscribe race.

If stream setup, reading, or the initial list fails, the frontend retries with
bounded exponential backoff. Every reconnect repeats the full
`ready -> List -> changes` sequence, which repairs state after any missed event.
Cancellation during app shutdown stops the loop without surfacing a user error.

## Frontend State Application

All collection and `ServerItem` mutations use the existing `RunOnUI` pattern.

An `upsert` event:

- Updates the existing `ServerItem` in place when its ID is present, preserving
  cached view models and active streams.
- Creates one `ServerItem` and navigation entry when the ID is new.
- Updates progress-tracker state using the same status mapping as the current
  list merge.
- Reorders `Servers` and `NavigationItems` by `sort_order` after applying the
  changed item.
- Recomputes aggregate server statistics.

A `deleted` event removes the matching item from both collections and
recomputes aggregate statistics. Deleting an unknown ID is an idempotent no-op.

The initial `List` continues to use full reconciliation so it can remove stale
items after reconnection. Explicit refresh remains available as a user command,
but create, update, start, stop, and delete commands no longer trigger automatic
full-list refreshes; their results arrive through the change stream.

## Error and Backpressure Behavior

- Subscriber overflow terminates only the slow subscription and never blocks
  or fails the store mutation that produced the event.
- RPC send failures and context cancellation remove the subscriber promptly.
- Unknown protobuf event variants are ignored for forward compatibility.
- Reconnection uses a bounded delay and does not create overlapping stream
  loops.
- The existing structured gRPC error conventions remain unchanged for unary
  server operations.

## Testing

Daemon tests will verify:

- A new watcher receives `ready` but no initial server snapshots.
- Each successful `Put` produces exactly one `upsert` containing the changed
  server.
- Each successful `Delete` produces exactly one `deleted` event.
- Failed persistence produces no event.
- Lifecycle transitions and exit reconciliation are observable through the
  same broker.
- Cancelling a stream unsubscribes it.
- A slow subscriber is disconnected without blocking publishers or other
  subscribers.

Frontend tests will extract or exercise single-event application independently
of WinUI rendering and verify:

- Upsert updates in place, inserts new items, maintains sort order, and updates
  progress/statistics.
- Delete removes exactly one item and tolerates an unknown ID.
- Initial list reconciliation followed by buffered changes leaves the newest
  state visible.
- Reconnection performs a new list reconciliation.

Verification includes protobuf regeneration, all Go tests, the C# core tests,
and an x64 WinUI build so generated client APIs and XAML compilation remain
compatible.

## Non-Goals

- Durable event history or cross-process replay.
- Sending an initial snapshot through the stream.
- Combining multiple changed servers into one event.
- Generalizing the broker to unrelated domain entities.
