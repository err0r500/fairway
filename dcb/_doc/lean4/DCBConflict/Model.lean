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

-- Read target: bucket + afterVersion
structure ReadTarget where
  bucket : Bucket
  afterVersion : Version

end DCBConflict
