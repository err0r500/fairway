import DCBConflict.Model
import DCBConflict.Operations
import DCBConflict.Matching

namespace DCBConflict

/-- The type bucket is always in writeBuckets (it's the head of the list). -/
theorem typeBucket_mem_writeBuckets (e : Event) :
    Bucket.typeBucket e.type ∈ (writeBuckets e).toList := by
  simp only [writeBuckets, NonEmptyList.toList]
  exact List.Mem.head _

/-- Membership in toList implies contains returns true. -/
theorem NonEmptyList.mem_toList_contains {α : Type} [BEq α] [LawfulBEq α]
    (nel : NonEmptyList α) (x : α) (h : x ∈ nel.toList) : nel.contains x = true := by
  simp only [NonEmptyList.contains, NonEmptyList.toList, List.mem_cons] at *
  cases h with
  | inl h => simp only [h, beq_self_eq_true, Bool.true_or]
  | inr h => simp only [List.elem_iff.mpr h, Bool.or_true]

/-- Contains returning true implies membership in toList (converse of mem_toList_contains). -/
theorem NonEmptyList.contains_mem_toList {α : Type} [BEq α] [LawfulBEq α]
    (nel : NonEmptyList α) (x : α) (h : nel.contains x = true) : x ∈ nel.toList := by
  simp only [NonEmptyList.contains, Bool.or_eq_true, beq_iff_eq] at h
  simp only [NonEmptyList.toList]
  cases h with
  | inl h => rw [← h]; exact List.Mem.head _
  | inr h => exact List.Mem.tail _ (List.elem_iff.mp h)

/-- Elements of any powerset member are contained in the original list. -/
theorem powerset_subset {α : Type} (xs : List α) (s : NonEmptyList α) (hs : s ∈ nonEmptyPowerset xs) :
    ∀ x ∈ s.toList, x ∈ xs := by
  induction xs generalizing s with
  | nil => simp [nonEmptyPowerset] at hs
  | cons y ys ih =>
    intro x hx
    simp only [nonEmptyPowerset] at hs
    have hs' := List.mem_append.mp hs
    rcases hs' with hs_left | hs_right
    · -- s ∈ ⟨y, []⟩ :: (nonEmptyPowerset ys).map ...
      rw [List.mem_cons] at hs_left
      rcases hs_left with rfl | hs_map
      · -- s = ⟨y, []⟩
        simp only [NonEmptyList.toList, List.mem_cons, List.mem_nil_iff] at hx
        rcases hx with rfl | h
        · exact List.Mem.head _
        · exact h.elim
      · -- s ∈ (nonEmptyPowerset ys).map ...
        simp only [List.mem_map] at hs_map
        obtain ⟨s', hs', rfl⟩ := hs_map
        simp only [NonEmptyList.toList, List.mem_cons] at hx
        rcases hx with rfl | rfl | hx'
        · exact List.Mem.head _
        · exact List.Mem.tail _ (ih s' hs' s'.head (List.Mem.head _))
        · exact List.Mem.tail _ (ih s' hs' x (List.Mem.tail _ hx'))
    · -- s ∈ nonEmptyPowerset ys
      exact List.Mem.tail _ (ih s hs_right x hx)

/-- NonEmptyList.contains is equivalent to List.contains on toList. -/
theorem NonEmptyList.contains_eq_toList_contains {α : Type} [BEq α] [LawfulBEq α]
    (nel : NonEmptyList α) (x : α) : nel.contains x = nel.toList.contains x := by
  simp only [NonEmptyList.contains, NonEmptyList.toList, List.contains, List.elem]
  cases hbeq : x == nel.head with
  | true =>
    have heq : nel.head = x := (beq_iff_eq.mp hbeq).symm
    simp [heq]
  | false =>
    have hne : nel.head ≠ x := fun h => by rw [h, beq_self_eq_true] at hbeq; contradiction
    simp [hne]

/-- If all elements of a NonEmptyList are in xs, there exists a sameElements-equivalent
    subset in the powerset of xs. Key lemma for completeness. -/
theorem subset_in_powerset {α : Type} [BEq α] [LawfulBEq α]
    (xs : List α) (nel : NonEmptyList α) (h : ∀ x ∈ nel.toList, x ∈ xs) :
    ∃ s ∈ nonEmptyPowerset xs, nel.sameElements s = true := by
  induction xs generalizing nel with
  | nil =>
    have := h nel.head (List.Mem.head _)
    simp at this
  | cons y ys ih =>
    by_cases hy : y ∈ nel.toList
    · -- y ∈ nel
      by_cases hall : ∀ x ∈ nel.toList, x = y
      · -- nel contains only y
        refine ⟨⟨y, []⟩, ?_, ?_⟩
        · simp [nonEmptyPowerset]
        · simp only [NonEmptyList.sameElements, Bool.and_eq_true, List.all_eq_true]
          constructor
          · intro x hx
            simp only [NonEmptyList.toList, List.contains, List.elem]
            rw [hall x hx]
            simp
          · intro x hx
            simp only [NonEmptyList.toList, List.mem_singleton] at hx
            rw [hx, ← NonEmptyList.contains_eq_toList_contains]
            exact NonEmptyList.mem_toList_contains nel y hy
      · -- nel has elements other than y
        have ⟨z, hz_nel, hz_ne⟩ : ∃ z, z ∈ nel.toList ∧ z ≠ y :=
          Classical.byContradiction fun hcontra =>
            hall fun x hx =>
              Classical.byContradiction fun hxy =>
                hcontra ⟨x, hx, hxy⟩
        have hz_ys : z ∈ ys := by
          have := h z hz_nel
          simp only [List.mem_cons] at this
          exact this.resolve_left hz_ne
        -- Define nel' as the elements of nel that are not y
        let elems' := nel.toList.filter (· != y)
        have helems'_ne : elems' ≠ [] := by
          intro heq
          have hz_mem : z ∈ elems' := List.mem_filter.mpr ⟨hz_nel, by simp [bne, hz_ne]⟩
          rw [heq] at hz_mem
          simp at hz_mem
        obtain ⟨hd, tl, helems'_eq⟩ : ∃ hd tl, elems' = hd :: tl := by
          cases helems' : elems' with
          | nil => exact absurd helems' helems'_ne
          | cons hd tl => exact ⟨hd, tl, rfl⟩
        let nel' : NonEmptyList α := ⟨hd, tl⟩
        have hnel'_sub : ∀ x ∈ nel'.toList, x ∈ ys := by
          intro x hx
          simp only [nel', NonEmptyList.toList] at hx
          rw [← helems'_eq] at hx
          have ⟨hx_nel, hx_ne⟩ := List.mem_filter.mp hx
          have := h x hx_nel
          simp only [List.mem_cons] at this
          rcases this with rfl | hys
          · simp [bne] at hx_ne
          · exact hys
        obtain ⟨s', hs'_mem, hs'_same⟩ := ih nel' hnel'_sub
        refine ⟨⟨y, s'.head :: s'.tail⟩, ?_, ?_⟩
        · -- Show ⟨y, s'.head :: s'.tail⟩ ∈ nonEmptyPowerset (y :: ys)
          simp only [nonEmptyPowerset]
          apply List.Mem.tail
          apply List.mem_append_left
          exact List.mem_map.mpr ⟨s', hs'_mem, rfl⟩
        · simp only [NonEmptyList.sameElements, Bool.and_eq_true, List.all_eq_true]
          constructor
          · intro x hx
            simp only [NonEmptyList.toList, List.contains, List.elem]
            by_cases hxy : x = y
            · simp [hxy]
            · have hx_nel' : x ∈ nel'.toList := by
                simp only [nel', NonEmptyList.toList, ← helems'_eq]
                exact List.mem_filter.mpr ⟨hx, by simp [bne, hxy]⟩
              simp only [NonEmptyList.sameElements, Bool.and_eq_true, List.all_eq_true] at hs'_same
              have hmem := hs'_same.1 x hx_nel'
              simp only [List.contains, List.elem_iff] at hmem
              simp only [NonEmptyList.toList, List.mem_cons] at hmem
              cases hh : x == y with
              | true => simp at hh; exact absurd hh hxy
              | false =>
                cases hs : x == s'.head with
                | true => rfl
                | false =>
                  have hne : x ≠ s'.head := fun h => by rw [h, beq_self_eq_true] at hs; contradiction
                  exact List.elem_iff.mpr (hmem.resolve_left hne)
          · intro x hx
            simp only [NonEmptyList.toList] at hx
            rw [List.mem_cons, List.mem_cons] at hx
            rcases hx with rfl | rfl | hx_tail
            · rw [← NonEmptyList.contains_eq_toList_contains]
              exact NonEmptyList.mem_toList_contains nel _ hy
            · simp only [NonEmptyList.sameElements, Bool.and_eq_true, List.all_eq_true] at hs'_same
              have h1 := hs'_same.2 s'.head (List.Mem.head _)
              have hx_nel' : s'.head ∈ nel'.toList := List.elem_iff.mp h1
              simp only [nel', NonEmptyList.toList, ← helems'_eq] at hx_nel'
              have hmem := (List.mem_filter.mp hx_nel').1
              rw [← NonEmptyList.contains_eq_toList_contains]
              exact NonEmptyList.mem_toList_contains nel s'.head hmem
            · simp only [NonEmptyList.sameElements, Bool.and_eq_true, List.all_eq_true] at hs'_same
              have h1 := hs'_same.2 x (List.Mem.tail _ hx_tail)
              have hx_nel' : x ∈ nel'.toList := List.elem_iff.mp h1
              simp only [nel', NonEmptyList.toList, ← helems'_eq] at hx_nel'
              have hmem := (List.mem_filter.mp hx_nel').1
              rw [← NonEmptyList.contains_eq_toList_contains]
              exact NonEmptyList.mem_toList_contains nel x hmem
    · -- y ∉ nel
      have h' : ∀ x ∈ nel.toList, x ∈ ys := by
        intro x hx
        have := h x hx
        simp only [List.mem_cons] at this
        exact this.resolve_left (fun heq => hy (heq ▸ hx))
      obtain ⟨s, hs, hsame⟩ := ih nel h'
      refine ⟨s, ?_, hsame⟩
      simp only [nonEmptyPowerset]
      apply List.Mem.tail
      exact List.mem_append_right _ hs

/-- If tags are sameElements with a powerset member s, then tags is a subset
    of the original list. Key lemma for precision. -/
theorem sameElements_powerset_subsetOf {α : Type} [BEq α] [LawfulBEq α]
    (tags s : NonEmptyList α) (xs : List α)
    (hs_pow : s ∈ nonEmptyPowerset xs)
    (hs_same : tags.sameElements s = true) :
    tags.subsetOf xs = true := by
  simp only [NonEmptyList.subsetOf, Bool.and_eq_true]
  simp only [NonEmptyList.sameElements, Bool.and_eq_true, List.all_eq_true] at hs_same
  have hpow := powerset_subset xs s hs_pow
  constructor
  · -- xs.contains tags.head
    have h1 := hs_same.1 tags.head (List.Mem.head _)
    have h2 := List.elem_iff.mp h1
    have h3 := hpow s.head (List.Mem.head _)
    -- h2 : tags.head ∈ s.toList
    -- Need to show tags.head ∈ xs
    exact List.elem_iff.mpr (hpow tags.head (List.elem_iff.mp h1))
  · -- tags.tail.all xs.contains
    apply List.all_eq_true.mpr
    intro x hx
    have h1 := hs_same.1 x (List.Mem.tail _ hx)
    exact List.elem_iff.mpr (hpow x (List.elem_iff.mp h1))

end DCBConflict
