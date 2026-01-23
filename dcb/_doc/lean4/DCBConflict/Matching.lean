import DCBConflict.Model

namespace DCBConflict

-- Membership for NonEmptyList
def NonEmptyList.contains {α : Type} [BEq α] (s : NonEmptyList α) (x : α) : Bool :=
  s.head == x || s.tail.contains x

instance {α : Type} [BEq α] : Membership α (NonEmptyList α) where
  mem elem container := NonEmptyList.contains elem container

-- Subset: all elements of NonEmptyList are in the target List
def NonEmptyList.subsetOf {α : Type} [BEq α] (a : NonEmptyList α) (b : List α) : Bool :=
  b.contains a.head && a.tail.all b.contains

-- Event matches a query item
def matchesItem (e : Event) (item : QueryItem) : Bool :=
  match item with
  | .typeOnly types => types.contains e.type
  | .tagsOnly tags => tags.subsetOf e.tags
  | .typesAndTags types tags => types.contains e.type && tags.subsetOf e.tags

-- Event matches query: version > afterVersion AND matches some item
def matchesQuery (e : Event) (q : Query) : Prop :=
  e.version > q.afterVersion ∧
  ∃ item ∈ q.items.toList, matchesItem e item

instance (e : Event) (q : Query) : Decidable (matchesQuery e q) :=
  if h : e.version > q.afterVersion ∧ q.items.toList.any (matchesItem e) then
    isTrue ⟨h.1, List.any_eq_true.mp h.2⟩
  else
    isFalse fun ⟨h1, h2⟩ => h ⟨h1, List.any_eq_true.mpr h2⟩

end DCBConflict
