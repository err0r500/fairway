import DCBConflict.Basic
import Std.Data.HashSet.Lemmas

namespace DCBConflict

-- Subset for HashSet
def HashSet.subset (a b : Std.HashSet Tag) : Prop := ∀ x ∈ a, x ∈ b
infix:50 " ⊆ₕ " => HashSet.subset

-- Decidable via all
instance (a b : Std.HashSet Tag) : Decidable (a ⊆ₕ b) :=
  if h : a.all (b.contains ·) then
    isTrue (fun x hx => Std.HashSet.mem_iff_contains.mpr
      (Std.HashSet.all_eq_true_iff_forall_mem.mp h x hx))
  else
    isFalse (fun hsub => h (Std.HashSet.all_eq_true_iff_forall_mem.mpr
      (fun x hx => Std.HashSet.mem_iff_contains.mp (hsub x hx))))

-- Membership for NonEmptyHashSet
def NonEmptyHashSet.contains {α : Type} [BEq α] [Hashable α] (s : NonEmptyHashSet α) (x : α) : Bool :=
  s.head == x || s.rest.contains x

  instance {α : Type} [BEq α] [Hashable α] : Membership α (NonEmptyHashSet α) where
    mem x s := x.contains s

-- Subset for NonEmptyHashSet to HashSet
def NonEmptyHashSet.subsetOf {α : Type} [BEq α] [Hashable α] (a : NonEmptyHashSet α) (b : Std.HashSet α) : Bool :=
  b.contains a.head && a.rest.all b.contains

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
