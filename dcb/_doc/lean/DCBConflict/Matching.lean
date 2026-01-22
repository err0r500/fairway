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

-- Event matches a query item
def matchesItem (e : Event) (item : QueryItem) : Prop :=
  (item.types.isEmpty ∨ e.type ∈ item.types) ∧
  item.tags ⊆ₕ e.tags

instance (e : Event) (item : QueryItem) : Decidable (matchesItem e item) :=
  inferInstanceAs (Decidable (_ ∧ _))

-- Event matches query: version > afterVersion AND matches some item
def matchesQuery (e : Event) (q : Query) : Prop :=
  e.version > q.afterVersion ∧
  ∃ item ∈ q.items, matchesItem e item

instance (e : Event) (q : Query) : Decidable (matchesQuery e q) :=
  inferInstanceAs (Decidable (_ ∧ _))

end DCBConflict
