import DCBConflict.Model
import DCBConflict.Matching

namespace DCBConflict

-- Check if two NonEmptyHashSets have the same elements
def NonEmptyHashSet.sameElements {α : Type} [BEq α] [Hashable α]
    (a b : NonEmptyHashSet α) : Bool :=
  a.toList.all (b.contains ·) && b.toList.all (a.contains ·)

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


-- Read targets for a query item (uses event's powerset for structural equality)
def readTargetsItem (item : QueryItem) (afterVersion : Version) (e : Event) : List ReadTarget :=
  match item with
  | .typeOnly types => types.toList.map fun t => ⟨Bucket.typeBucket t, afterVersion⟩
  | .tagsOnly tags =>
      (nonEmptyPowerset e.tags.toList).filter (tags.sameElements ·)
        |>.map fun s => ⟨Bucket.tagBucket e.type s, afterVersion⟩
  | .typesAndTags types tags =>
      (nonEmptyPowerset e.tags.toList).filter (tags.sameElements ·)
        |>.flatMap fun s => types.toList.map fun t => ⟨Bucket.tagBucket t s, afterVersion⟩

-- Read targets for entire query
def readTargets (q : Query) (e : Event) : List ReadTarget :=
  q.items.toList.flatMap (readTargetsItem · q.afterVersion e)

-- Conflict: bucket matches AND version > afterVersion
def conflictDetected (e : Event) (q : Query) : Prop :=
  ∃ wb ∈ (writeBuckets e).toList, ∃ rt ∈ readTargets q e,
    wb = rt.bucket ∧ e.version > rt.afterVersion

end DCBConflict
