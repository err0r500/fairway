import DCBConflict.Basic
import DCBConflict.Matching

namespace DCBConflict

-- Powerset of a list
def listPowerset {α : Type} : List α → List (List α)
  | [] => [[]]
  | x :: xs =>
    let rest := listPowerset xs
    rest ++ rest.map (x :: ·)

-- Write buckets: one per non-empty tag subset
def writeBuckets (e : Event) : List Bucket :=
  (listPowerset e.tags.toList).filter (· ≠ []) |>.map fun subset =>
    { tags := Std.ExtHashSet.ofList subset, type := some e.type }

-- Read targets for a query item
def readTargetsItem (item : QueryItem) (afterVersion : Version) : List ReadTarget :=
  if item.types.isEmpty then
    [{ bucket := { tags := Std.ExtHashSet.ofList item.tags.toList, type := none }, afterVersion }]
  else
    item.types.toList.map fun t =>
      { bucket := { tags := Std.ExtHashSet.ofList item.tags.toList, type := some t }, afterVersion }

-- Read targets for entire query
def readTargets (q : Query) : List ReadTarget :=
  q.items.flatMap (readTargetsItem · q.afterVersion)

-- Conflict: bucket matches AND version > afterVersion
def conflictDetected (e : Event) (q : Query) : Prop :=
  ∃ wb ∈ writeBuckets e, ∃ rt ∈ readTargets q,
    wb = rt.bucket ∧ e.version > rt.afterVersion

end DCBConflict
