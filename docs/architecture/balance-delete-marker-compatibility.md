# Balance delete-marker compatibility

The current release uses `balance_delete_marker:{transactions}:<balance>` as the
dedicated marker namespace. During the compatibility window, delete commands also
write the pre-namespace `<balance>:deleted` marker with the same owner token. Both
transaction Lua scripts reject either marker, so normal old/new operations cannot
cross a live delete barrier. The sections below state what that bridge guarantees
in a mixed fleet and the one hazard it does not cover.

The legacy suffix has an intentional collision tradeoff, because a balance key can
itself contain `:`. Three effects follow from a valid sibling balance whose key is
`<balance>:deleted`. While that sibling is cached it occupies the old marker key,
so dual acquisition fails closed and rolls back the namespaced marker: the delete
of `<balance>` is rejected, but no unsafe delete is allowed. While that sibling is
cached, the legacy Lua check also rejects every transaction on `<balance>` with
`0019`. When the sibling is not cached, the legacy `SetNX` succeeds and writes the
owner token into the sibling's own cache key, poisoning it for the marker lifetime:
30 seconds after a successful eviction, up to 48 hours when eviction failed. All
three effects belong to the suffix itself, pre-date this release, and disappear
with the bridge. The dedicated namespace is collision-free.

The two `SET NX` calls are deliberately kept behind the same lease and the
delete proceeds only after both succeed, but they are still separate Redis
round-trips. There is a small acquisition interval in which only one marker is
present; a future multi-key Lua acquisition can remove that interval if the
deployment needs a stronger guarantee. Do not describe this one-release bridge
as atomic cross-namespace locking.

Marker lifetime is conditional. Acquisition uses the long snapshot-protection TTL
of 48 hours. If cache eviction fails, that TTL is retained so a stale snapshot
cannot be used after the delete. If eviction succeeds, `ExpireIfValue` shortens
both owned markers atomically to the 30-second in-flight window; there is no
release/reacquire gap. Cascade deletes report one eviction status per balance and
shorten only the markers for balances whose cache eviction succeeded.

## Rolling deploy

A normal rolling deploy is supported. The bridge covers both directions of a mixed
fleet: old Lua honors the legacy `<balance>:deleted` marker that new pods
dual-write, and new Lua honors the namespaced marker and the legacy one. A delete
issued by a new pod raises a barrier that old transaction scripts respect, and a
delete issued by an old pod raises a barrier that new transaction scripts respect.

One residual hazard remains, and it requires every one of these conditions at once:

1. an old-binary delete stays in flight longer than its 30-second legacy marker
   TTL, so its marker expires while the request is still running;
2. a new pod acquires that same legacy key after it expires and owns it;
3. the old request then rolls back and releases with its unconditional `DEL`,
   removing the new pod's legacy marker — new code cannot make an already-running
   old binary compare an owner token; and
4. a third old pod mutates the balance in the window between the new pod's
   snapshot read and its cache `Del`, because the namespaced marker still exists
   but old Lua checks only `:deleted`.

Assess this as negligible. It needs a delete slower than 30 seconds, a legacy
expiry landing inside it, a rollback after that expiry, and a concurrent old-pod
mutation inside a window of a few Redis round-trips. Weigh it against the baseline
of any rollout: old binaries still carry the original defect for the deletes they
issue themselves, regardless of markers. The bridge closes the window only for
deletes issued by new pods.

## Rolling back

Rolling back to the previous release is allowed. What is lost is the namespaced
barrier: `balance_delete_marker:{transactions}:<balance>` is invisible to the old
binary, so a stale snapshot left by a failed eviction can be mutated by it. That is
the pre-existing defect class, not a new one, and its exposure is bounded by the
snapshot TTL.

During either handoff, monitor marker rejections (`0019`), marker release and
shortening warnings, and failed cache evictions. Failed evictions are the signal
that stale snapshots are being left behind.

## Legacy bridge retirement

Remove the legacy bridge only after both gates are satisfied:

- every old binary is retired, including as a rollback target; and
- one long marker TTL (`balanceDeleteMarkerTTLSeconds`, 48 hours) has elapsed
  since the last bridge write.

No Redis scan is required. Legacy markers always carry a TTL — 48 hours on
acquisition, 30 seconds after a successful eviction — so they expire on their own.

Then remove the legacy `SetNX`, the legacy release and expiry calls, the legacy
checks from both Lua scripts, and the legacy helper and fields. Retain the
dedicated namespace and the token-checked operations. Keep the rollback target
namespace-aware; do not reintroduce a legacy-only rollback path.
