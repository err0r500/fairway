import DCBConflict.Basic
import DCBConflict.Matching

namespace DCBConflict

-- Non-empty powerset: all non-empty subsets
def nonEmptyPowerset {α : Type} [BEq α] [Hashable α] : List α → List (NonEmptyHashSet α)
  | [] => []
  | x :: xs =>
    let rest := nonEmptyPowerset xs
    let withX := rest.map fun s => ⟨x, s.rest.insert s.head⟩
    ⟨x, {}⟩ :: withX ++ rest

-- Write buckets: typeBucket + one tagBucket per non-empty tag subset
def writeBuckets (e : Event) : NonEmptyHashSet Bucket :=
  let tagBuckets := (nonEmptyPowerset e.tags.toList).map (Bucket.tagBucket e.type ·)
  ⟨Bucket.typeBucket e.type, Std.HashSet.ofList tagBuckets⟩


-- Read targets for a query item
def readTargetsItem (item : QueryItem) (afterVersion : Version) : List ReadTarget :=
  match item with
  | .typeOnly types => types.toList.map fun t => ⟨Bucket.typeBucket t, afterVersion⟩
  | .tagsOnly _ => [] -- tags-only can't create concrete buckets without type
  | .typesAndTags types tags => types.toList.map fun t => ⟨Bucket.tagBucket t tags, afterVersion⟩

-- Read targets for entire query
def readTargets (q : Query) : List ReadTarget :=
  q.items.toList.flatMap (readTargetsItem · q.afterVersion)

-- Conflict: bucket matches AND version > afterVersion
def conflictDetected (e : Event) (q : Query) : Prop :=
  ∃ wb ∈ (writeBuckets e).toList, ∃ rt ∈ readTargets q,
    wb = rt.bucket ∧ e.version > rt.afterVersion

end DCBConflict
