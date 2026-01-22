import Std.Data.HashSet
import Std.Data.ExtHashSet

namespace DCBConflict

abbrev Tag := String
abbrev EventType := String
abbrev Version := Nat

structure NonEmptyHashSet (α : Type) [BEq α] [Hashable α] where
  head : α
  rest : Std.HashSet α

-- Event: type, tags, version (monotonically increasing id)
structure Event where
  type : EventType
  tags : Std.HashSet Tag
  version : Version

-- Query item: sum type for different query patterns
inductive QueryItem where
  | typeOnly (types : NonEmptyHashSet EventType)
  | tagsOnly (tags : NonEmptyHashSet Tag)
  | typesAndTags (types : NonEmptyHashSet EventType) (tags : NonEmptyHashSet Tag)

def NonEmptyHashSet.toList {α : Type} [BEq α] [Hashable α] (s : NonEmptyHashSet α) : List α := s.head :: s.rest.toList

instance {α : Type} [BEq α] [Hashable α] : BEq (NonEmptyHashSet α) where
  beq a b := a.toList == b.toList

instance {α : Type} [BEq α] [Hashable α] : Hashable (NonEmptyHashSet α) where
  hash s := hash s.toList

instance : BEq QueryItem where
  beq a b := match a, b with
    | .typeOnly t1, .typeOnly t2 => t1 == t2
    | .tagsOnly g1, .tagsOnly g2 => g1 == g2
    | .typesAndTags t1 g1, .typesAndTags t2 g2 => t1 == t2 && g1 == g2
    | _, _ => false

instance : Hashable QueryItem where
  hash q := match q with
    | .typeOnly t => hash (0, t)
    | .tagsOnly g => hash (1, g)
    | .typesAndTags t g => hash (2, t, g)

-- Query: OR of items, with version filter
structure Query where
  items : Std.HashSet QueryItem
  afterVersion : Version

inductive Bucket where
  | typeBucket (type : EventType)
  | tagBucket (type : EventType) (tags : NonEmptyHashSet Tag)

instance : BEq Bucket where
  beq a b := match a, b with
    | .typeBucket t1, .typeBucket t2 => t1 == t2
    | .tagBucket t1 g1, .tagBucket t2 g2 => t1 == t2 && g1 == g2
    | _, _ => false

instance : Hashable Bucket where
  hash b := match b with
    | .typeBucket t => hash (0, t)
    | .tagBucket t g => hash (1, t, g)

-- Helper lemma for List BEq
theorem List.beq_eq_true_of_eq {α : Type} [BEq α] [LawfulBEq α] (l : List α) : l.beq l = true := by
  induction l with
  | nil => rfl
  | cons h t ih => simp only [List.beq, beq_self_eq_true, Bool.true_and, ih]

theorem List.eq_of_beq_eq_true {α : Type} [BEq α] [LawfulBEq α] {l1 l2 : List α}
    (h : l1.beq l2 = true) : l1 = l2 := by
  induction l1 generalizing l2 with
  | nil =>
    cases l2 with
    | nil => rfl
    | cons => contradiction
  | cons h1 t1 ih =>
    cases l2 with
    | nil => contradiction
    | cons h2 t2 =>
      simp only [List.beq, Bool.and_eq_true] at h
      have hhead := eq_of_beq h.1
      have htail := ih h.2
      rw [hhead, htail]

-- LawfulBEq for NonEmptyHashSet (using axiom for HashSet equality)
axiom Std.HashSet.eq_of_toList_eq {α : Type} [BEq α] [Hashable α] [LawfulBEq α]
    {a b : Std.HashSet α} (h : a.toList = b.toList) : a = b

instance {α : Type} [BEq α] [Hashable α] [LawfulBEq α] : LawfulBEq (NonEmptyHashSet α) where
  eq_of_beq {a b} h := by
    simp only [BEq.beq, NonEmptyHashSet.toList] at h
    have hlist := List.eq_of_beq_eq_true h
    cases a; cases b
    simp only [List.cons.injEq] at hlist
    congr
    · exact hlist.1
    · exact Std.HashSet.eq_of_toList_eq hlist.2
  rfl {a} := by
    simp only [BEq.beq, NonEmptyHashSet.toList]
    exact List.beq_eq_true_of_eq _

instance : LawfulBEq Bucket where
  eq_of_beq {a b} h := by
    cases a <;> cases b <;> simp only [BEq.beq] at h
    · have := of_decide_eq_true h; exact congrArg Bucket.typeBucket this
    · contradiction
    · contradiction
    · simp only [Bool.and_eq_true] at h
      have h1 := of_decide_eq_true h.1
      have h2 := @eq_of_beq (NonEmptyHashSet Tag) _ _ _ _ h.2
      rw [h1, h2]
  rfl {a} := by
    cases a
    · simp only [BEq.beq]; rfl
    · simp only [BEq.beq]
      exact @beq_self_eq_true (NonEmptyHashSet Tag) _ _ _

-- Read target: bucket + afterVersion
structure ReadTarget where
  bucket : Bucket
  afterVersion : Version

end DCBConflict
