import DCBConflict.Basic
import DCBConflict.Matching
import DCBConflict.Index

namespace DCBConflict

-- Helper: subset appears in powerset
-- theorem subset_in_listPowerset {α : Type} (xs ys : List α) (h : ∀ x ∈ xs, x ∈ ys) :
--     xs ∈ listPowerset ys := by
--   sorry

-- Completeness: if event matches query, conflict is detected
theorem completeness (e : Event) (q : Query) (h : matchesQuery e q) : conflictDetected e q := by
  obtain ⟨hv, item, hitem, hmatch⟩ := h
  unfold conflictDetected
  sorry

-- Precision: if conflict detected, event matches query
theorem precision (e : Event) (q : Query) (h : conflictDetected e q) : matchesQuery e q := by
  obtain ⟨wb, hwb, rt, hrt, heq, hver⟩ := h
  unfold matchesQuery
  constructor
  · sorry
  · sorry

-- Main theorem
theorem conflict_iff_matches (e : Event) (q : Query) : matchesQuery e q ↔ conflictDetected e q :=
  ⟨completeness e q, precision e q⟩

end DCBConflict
