import Std.Data.HashSet
import Std.Data.ExtHashSet

namespace DCBConflict

abbrev Tag := String
abbrev EventType := String
abbrev Version := Nat

-- Event uses HashSet (has toList)
structure Event where
  type : EventType
  tags : Std.HashSet Tag
  version : Version

-- Query item uses HashSet (needs toList for iteration)
structure QueryItem where
  types : Std.HashSet EventType
  tags : Std.HashSet Tag

-- Query: OR of items, with version filter
structure Query where
  items : List QueryItem
  afterVersion : Version

-- Bucket uses ExtHashSet (extensional equality)
structure Bucket where
  tags : Std.ExtHashSet Tag
  type : Option EventType

-- Read target: bucket + afterVersion
structure ReadTarget where
  bucket : Bucket
  afterVersion : Version

end DCBConflict
