import Std.Data.HashSet

namespace DCBConflict

abbrev Tag := String
abbrev EventType := String
abbrev Version := Nat

structure NonEmptyList (α : Type) where
  head : α
  tail : List α

def NonEmptyList.toList {α : Type} (s : NonEmptyList α) : List α := s.head :: s.tail

instance {α : Type} [BEq α] : BEq (NonEmptyList α) where
  beq a b := a.head == b.head && a.tail == b.tail

instance {α : Type} [Hashable α] : Hashable (NonEmptyList α) where
  hash s := hash s.toList

-- Event: type, tags, version (monotonically increasing id)
structure Event where
  type : EventType
  tags : List Tag
  version : Version

-- Query item: sum type for different query patterns
inductive QueryItem where
  | typeOnly (types : NonEmptyList EventType)
  | tagsOnly (tags : NonEmptyList Tag)
  | typesAndTags (types : NonEmptyList EventType) (tags : NonEmptyList Tag)

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
  | tagBucket (type : EventType) (tags : NonEmptyList Tag)

instance : BEq Bucket where
  beq a b := match a, b with
    | .typeBucket t1, .typeBucket t2 => t1 == t2
    | .tagBucket t1 g1, .tagBucket t2 g2 => t1 == t2 && g1 == g2
    | _, _ => false

instance : Hashable Bucket where
  hash b := match b with
    | .typeBucket t => hash (0, t)
    | .tagBucket t g => hash (1, t, g)

-- List.beq reflexivity
theorem List.beq_refl {α : Type} [BEq α] [LawfulBEq α] (l : List α) : l.beq l = true := by
  induction l with
  | nil => rfl
  | cons h t ih => simp only [List.beq, beq_self_eq_true, Bool.true_and, ih]

-- List.beq implies equality
theorem List.eq_of_beq' {α : Type} [BEq α] [LawfulBEq α] {l1 l2 : List α}
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
      simp only [eq_of_beq h.1, ih h.2]

-- For NonEmptyList: convert between head == and decide (head =)
theorem NonEmptyList.eq_of_components {α : Type} {a b : NonEmptyList α}
    (h1 : a.head = b.head) (h2 : a.tail = b.tail) : a = b := by
  cases a; cases b; simp only at h1 h2; simp only [h1, h2]

theorem NonEmptyList.eq_of_beq {α : Type} [BEq α] [inst : LawfulBEq α] {a b : NonEmptyList α}
    (h : (a == b) = true) : a = b := by
  have h' : (a.head == b.head && a.tail.beq b.tail) = true := h
  simp only [Bool.and_eq_true] at h'
  have h1 : a.head = b.head := @LawfulBEq.eq_of_beq α _ inst _ _ h'.1
  have h2 : a.tail = b.tail := List.eq_of_beq' h'.2
  exact NonEmptyList.eq_of_components h1 h2

theorem NonEmptyList.beq_rfl {α : Type} [BEq α] [LawfulBEq α] (a : NonEmptyList α) :
    (a == a) = true := by
  simp only [BEq.beq, beq_self_eq_true, List.beq_refl, Bool.and_self]

instance {α : Type} [BEq α] [LawfulBEq α] : LawfulBEq (NonEmptyList α) where
  eq_of_beq := NonEmptyList.eq_of_beq
  rfl := NonEmptyList.beq_rfl _

-- For Bucket: need to handle the expanded form where String uses decide
theorem NonEmptyList.eq_of_beq_expanded {a b : NonEmptyList Tag}
    (h1 : decide (a.head = b.head) = true) (h2 : a.tail.beq b.tail = true) : a = b := by
  have hhead : a.head = b.head := of_decide_eq_true h1
  have htail : a.tail = b.tail := List.eq_of_beq' h2
  exact NonEmptyList.eq_of_components hhead htail

theorem NonEmptyList.beq_rfl_expanded (a : NonEmptyList Tag) :
    a.tail.beq a.tail = true := List.beq_refl a.tail

instance : LawfulBEq Bucket where
  eq_of_beq {a b} h := by
    cases a <;> cases b <;> simp only [BEq.beq] at h
    · exact congrArg Bucket.typeBucket (of_decide_eq_true h)
    · contradiction
    · contradiction
    · simp only [Bool.and_eq_true] at h
      have h1 := of_decide_eq_true h.1
      have h2 := NonEmptyList.eq_of_beq_expanded h.2.1 h.2.2
      simp only [h1, h2]
  rfl {a} := by
    cases a
    · simp only [BEq.beq, decide_true]
    · simp only [BEq.beq, decide_true, Bool.true_and, NonEmptyList.beq_rfl_expanded]

-- Read target: bucket + afterVersion
structure ReadTarget where
  bucket : Bucket
  afterVersion : Version

end DCBConflict
