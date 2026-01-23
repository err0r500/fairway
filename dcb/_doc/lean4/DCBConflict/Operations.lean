import DCBConflict.Model
import DCBConflict.Matching

namespace DCBConflict

-- Check if two NonEmptyLists have the same elements (as sets)
def NonEmptyList.sameElements {α : Type} [BEq α]
    (a b : NonEmptyList α) : Bool :=
  a.toList.all (b.toList.contains ·) && b.toList.all (a.toList.contains ·)

-- Non-empty powerset: all non-empty subsets
def nonEmptyPowerset {α : Type} : List α → List (NonEmptyList α)
  | [] => []
  | x :: xs =>
    let rest := nonEmptyPowerset xs
    let withX := rest.map fun s => ⟨x, s.head :: s.tail⟩
    ⟨x, []⟩ :: withX ++ rest

-- Write buckets: typeBucket + one tagBucket per non-empty tag subset
def writeBuckets (e : Event) : NonEmptyList Bucket :=
  let tagBuckets := (nonEmptyPowerset e.tags).map (Bucket.tagBucket e.type ·)
  ⟨Bucket.typeBucket e.type, tagBuckets⟩

-- Read targets for a query item (uses event's powerset for structural equality)
def readTargetsItem (item : QueryItem) (afterVersion : Version) (e : Event) : List ReadTarget :=
  match item with
  | .typeOnly types => types.toList.map fun t => ⟨Bucket.typeBucket t, afterVersion⟩
  | .tagsOnly tags =>
      (nonEmptyPowerset e.tags).filter (tags.sameElements ·)
        |>.map fun s => ⟨Bucket.tagBucket e.type s, afterVersion⟩
  | .typesAndTags types tags =>
      (nonEmptyPowerset e.tags).filter (tags.sameElements ·)
        |>.flatMap fun s => types.toList.map fun t => ⟨Bucket.tagBucket t s, afterVersion⟩

-- Read targets for entire query
def readTargets (q : Query) (e : Event) : List ReadTarget :=
  q.items.toList.flatMap (readTargetsItem · q.afterVersion e)

-- Conflict: bucket matches AND version > afterVersion
def conflictDetected (e : Event) (q : Query) : Prop :=
  ∃ wb ∈ (writeBuckets e).toList, ∃ rt ∈ readTargets q e,
    wb = rt.bucket ∧ e.version > rt.afterVersion

end DCBConflict
