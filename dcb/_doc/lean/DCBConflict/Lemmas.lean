import DCBConflict.Model
import DCBConflict.Operations
import DCBConflict.Matching

/-!
# Helper Lemmas for DCB Conflict Detection

This module contains auxiliary lemmas used to prove the main theorems
(`completeness` and `precision`) in `DCBConflict.Theorems`.

## Overview

The conflict detection algorithm works by:
1. **Write side**: An event generates "write buckets" (type bucket + tag buckets for all tag subsets)
2. **Read side**: A query generates "read targets" based on its query items
3. **Conflict**: Detected when a write bucket matches a read target with appropriate version

These helpers establish the correspondence between:
- `NonEmptyHashSet.contains` and list membership
- Powerset construction and subset relationships
- `sameElements` equivalence for structural matching

## Sections

- **NonEmptyHashSet membership**: Bidirectional lemmas between `contains` and `toList`
- **WriteBuckets**: Lemmas about bucket membership in `writeBuckets e`
- **ReadTargets**: Lemmas about target membership in `readTargets q e`
- **Powerset**: Completeness and subset properties of `nonEmptyPowerset`
- **SameElements**: Reflexivity and transitivity through event tags
-/

namespace DCBConflict

/-! ## NonEmptyHashSet Membership Lemmas

These lemmas establish the equivalence between the boolean `contains` predicate
and list membership in `toList`. This is fundamental for converting between
computational checks and propositional reasoning.
-/

/-- If `s.contains x = true`, then `x` appears in `s.toList`.
    This is the "soundness" direction: contains implies membership. -/
theorem NonEmptyHashSet.contains_mem_toList {α : Type} [BEq α] [Hashable α] [LawfulBEq α]
    (s : NonEmptyHashSet α) (x : α) (h : s.contains x = true) :
    x ∈ s.toList := by
  simp only [NonEmptyHashSet.contains, Bool.or_eq_true, beq_iff_eq] at h
  simp only [NonEmptyHashSet.toList]
  cases h with
  | inl h => rw [h]; exact List.Mem.head _
  | inr h =>
    apply List.Mem.tail
    exact Std.HashSet.mem_toList.mpr (Std.HashSet.mem_iff_contains.mpr h)

/-- If `x` appears in `s.toList`, then `s.contains x = true`.
    This is the "completeness" direction: membership implies contains. -/
theorem NonEmptyHashSet.toList_mem_contains {α : Type} [BEq α] [Hashable α] [LawfulBEq α]
    (s : NonEmptyHashSet α) (x : α) (h : x ∈ s.toList) : s.contains x = true := by
  simp only [NonEmptyHashSet.contains, NonEmptyHashSet.toList, List.mem_cons] at *
  cases h with
  | inl h => simp only [h, beq_self_eq_true, Bool.true_or]
  | inr h => simp only [Std.HashSet.mem_iff_contains.mp (Std.HashSet.mem_toList.mp h), Bool.or_true]

/-- Equivalent formulation: `contains` implies `toList` membership (curried). -/
theorem NonEmptyHashSet.all_mem_toList {α : Type} [BEq α] [Hashable α] [LawfulBEq α]
    (s : NonEmptyHashSet α) : ∀ x, s.contains x → x ∈ s.toList := by
  intro x hx
  simp only [NonEmptyHashSet.contains, Bool.or_eq_true] at hx
  simp only [NonEmptyHashSet.toList]
  cases hx with
  | inl h => rw [eq_of_beq h]; exact List.Mem.head _
  | inr h => exact List.Mem.tail _ (Std.HashSet.mem_toList.mpr (Std.HashSet.mem_iff_contains.mpr h))

/-- `sameElements` is reflexive: every set has the same elements as itself. -/
theorem NonEmptyHashSet.sameElements_refl {α : Type} [BEq α] [Hashable α] [LawfulBEq α]
    (s : NonEmptyHashSet α) : s.sameElements s = true := by
  simp only [NonEmptyHashSet.sameElements, Bool.and_eq_true, List.all_eq_true]
  exact ⟨fun x hx => NonEmptyHashSet.toList_mem_contains s x hx,
         fun x hx => NonEmptyHashSet.toList_mem_contains s x hx⟩

/-! ## Singleton NonEmptyHashSet Lemmas

Properties of singleton sets `⟨x, {}⟩`, used when a query matches only one element.
-/

/-- A singleton set contains only elements equal to its head. -/
theorem singleton_contains {α : Type} [BEq α] [Hashable α] [LawfulBEq α] (x y : α) :
    (⟨x, {}⟩ : NonEmptyHashSet α).contains y = (x == y) := by
  simp only [NonEmptyHashSet.contains, Std.HashSet.contains_empty, Bool.or_false]

/-- A singleton set's toList is a single-element list. -/
theorem singleton_toList {α : Type} [BEq α] [Hashable α] (x : α) :
    (⟨x, {}⟩ : NonEmptyHashSet α).toList = [x] := by
  simp only [NonEmptyHashSet.toList, Std.HashSet.toList_empty]

/-! ## WriteBuckets Lemmas

These lemmas show which buckets appear in `writeBuckets e` for an event `e`.
An event generates:
- One `typeBucket e.type` (always present as head)
- One `tagBucket e.type s` for each non-empty subset `s` of `e.tags`
-/

/-- List membership transfers to HashSet.ofList membership. -/
theorem mem_hashset_ofList_toList {α : Type} [BEq α] [Hashable α] [LawfulBEq α]
    (x : α) (xs : List α) (h : x ∈ xs) :
    x ∈ (Std.HashSet.ofList xs).toList := by
  apply Std.HashSet.mem_toList.mpr
  apply Std.HashSet.mem_ofList.mpr
  exact List.elem_iff.mpr h

/-- The type bucket is always in writeBuckets (it's the head). -/
theorem typeBucket_in_writeBuckets (e : Event) :
    Bucket.typeBucket e.type ∈ (writeBuckets e).toList := by
  simp only [writeBuckets, NonEmptyHashSet.toList]
  exact List.Mem.head _

/-- Any tag bucket from the powerset is in writeBuckets. -/
theorem powerset_tagBucket_in_writeBuckets (e : Event) (s : NonEmptyHashSet Tag)
    (hs : s ∈ nonEmptyPowerset e.tags.toList) :
    Bucket.tagBucket e.type s ∈ (writeBuckets e).toList := by
  simp only [writeBuckets, NonEmptyHashSet.toList]
  apply List.Mem.tail
  have h : Bucket.tagBucket e.type s ∈ (nonEmptyPowerset e.tags.toList).map (Bucket.tagBucket e.type) := by
    simp only [List.mem_map]
    exact ⟨s, hs, rfl⟩
  exact mem_hashset_ofList_toList _ _ h

/-! ## ReadTargets Lemmas

These lemmas show which read targets appear in `readTargets q e`.
-/

/-- A read target from a matching query item is in the full readTargets list. -/
theorem readTarget_in_readTargets (q : Query) (e : Event) (item : QueryItem)
    (hitem : item ∈ q.items.toList) (rt : ReadTarget)
    (hrt : rt ∈ readTargetsItem item q.afterVersion e) :
    rt ∈ readTargets q e := by
  simp only [readTargets, List.mem_flatMap]
  exact ⟨item, hitem, hrt⟩

/-- All read targets inherit the query's afterVersion. -/
theorem readTarget_afterVersion (q : Query) (e : Event) (rt : ReadTarget) (item : QueryItem)
    (hrt : rt ∈ readTargetsItem item q.afterVersion e) :
    rt.afterVersion = q.afterVersion := by
  cases item <;> simp only [readTargetsItem, List.mem_map, List.mem_filter, List.mem_flatMap] at hrt
  · obtain ⟨_, _, rfl⟩ := hrt; rfl
  · obtain ⟨_, ⟨_, _⟩, rfl⟩ := hrt; rfl
  · obtain ⟨_, ⟨_, _⟩, _, _, rfl⟩ := hrt; rfl

/-! ## Powerset Lemmas

The `nonEmptyPowerset` function generates all non-empty subsets of a list.
These lemmas establish:
- **Soundness**: Elements of powerset are subsets of the original
- **Completeness**: Every non-empty subset appears (up to `sameElements`)
-/

/-- A singleton `{x}` is in the powerset if `x` is in the list. -/
theorem singleton_in_powerset {α : Type} [BEq α] [Hashable α] (x : α) (xs : List α)
    (hx : x ∈ xs) : ⟨x, {}⟩ ∈ nonEmptyPowerset xs := by
  induction xs with
  | nil => contradiction
  | cons y ys ih =>
    unfold nonEmptyPowerset
    cases hx with
    | head => apply List.mem_cons_self
    | tail _ h =>
      apply List.mem_cons_of_mem
      apply List.mem_append_right
      exact ih h

/-- Extending a powerset element with a new head preserves membership. -/
theorem withX_in_powerset {α : Type} [BEq α] [Hashable α] (x : α) (ys : List α)
    (s' : NonEmptyHashSet α) (hs' : s' ∈ nonEmptyPowerset ys) :
    ⟨x, s'.rest.insert s'.head⟩ ∈ nonEmptyPowerset (x :: ys) := by
  unfold nonEmptyPowerset
  apply List.mem_cons_of_mem
  apply List.mem_append_left
  simp only [List.mem_map]
  exact ⟨s', hs', rfl⟩

/-- Characterizes membership in an extended powerset element. -/
theorem withX_toList_mem {α : Type} [BEq α] [Hashable α] [LawfulBEq α]
    (x : α) (s' : NonEmptyHashSet α) (y : α) :
    y ∈ (⟨x, s'.rest.insert s'.head⟩ : NonEmptyHashSet α).toList ↔ y = x ∨ y ∈ s'.toList := by
  simp only [NonEmptyHashSet.toList, List.mem_cons]
  constructor
  · intro h
    cases h with
    | inl h => left; exact h
    | inr h =>
      right
      have h' := Std.HashSet.mem_toList.mp h
      cases Std.HashSet.mem_insert.mp h' with
      | inl heq => left; exact (eq_of_beq heq).symm
      | inr hmem => right; exact Std.HashSet.mem_toList.mpr hmem
  · intro h
    cases h with
    | inl h => left; exact h
    | inr h =>
      right
      apply Std.HashSet.mem_toList.mpr
      cases h with
      | inl h =>
        apply Std.HashSet.mem_insert.mpr
        left; rw [h]; exact beq_self_eq_true _
      | inr h =>
        apply Std.HashSet.mem_insert.mpr
        right; exact Std.HashSet.mem_toList.mp h

/-- **Soundness**: Every element of a powerset set is in the original list. -/
theorem powerset_elem_subset (xs : List Tag) (s : NonEmptyHashSet Tag)
    (hs : s ∈ nonEmptyPowerset xs) : ∀ x ∈ s.toList, x ∈ xs := by
  induction xs generalizing s with
  | nil => simp [nonEmptyPowerset] at hs
  | cons y ys ih =>
    unfold nonEmptyPowerset at hs
    cases List.mem_cons.mp hs with
    | inl h =>
      intro x hx
      rw [h, singleton_toList] at hx
      simp only [List.mem_singleton] at hx
      rw [hx]; exact List.Mem.head _
    | inr h =>
      cases List.mem_append.mp h with
      | inl h_withY =>
        simp only [List.mem_map] at h_withY
        obtain ⟨s', hs', hs_eq⟩ := h_withY
        intro x hx
        rw [← hs_eq] at hx
        simp only [NonEmptyHashSet.toList, List.mem_cons] at hx
        cases hx with
        | inl h => rw [h]; exact List.Mem.head _
        | inr h =>
          have h' := Std.HashSet.mem_toList.mp h
          cases Std.HashSet.mem_insert.mp h' with
          | inl heq =>
            have h_head : s'.head ∈ s'.toList := List.Mem.head _
            have h_in_ys := ih s' hs' s'.head h_head
            apply List.Mem.tail
            exact eq_of_beq heq ▸ h_in_ys
          | inr hmem =>
            have h_rest : x ∈ s'.rest.toList := Std.HashSet.mem_toList.mpr hmem
            have h_in_s' : x ∈ s'.toList := List.Mem.tail _ h_rest
            apply List.Mem.tail
            exact ih s' hs' x h_in_s'
      | inr hs_rest =>
        intro x hx
        apply List.Mem.tail
        exact ih s hs_rest x hx

/-! ## List Filtering Helpers

Utilities for filtering elements that appear in both lists.
-/

/-- Filter `ys` to elements that also appear in `xs`. -/
def filterIn {α : Type} [BEq α] (ys xs : List α) : List α :=
  ys.filter (xs.elem ·)

/-- Filtered elements are in the target list. -/
theorem filterIn_subset {α : Type} [BEq α] [LawfulBEq α] (ys xs : List α) :
    ∀ x ∈ filterIn ys xs, x ∈ xs := by
  intro x hx
  simp only [filterIn, List.mem_filter] at hx
  exact List.elem_iff.mp hx.2

/-- Filtered elements come from the source list. -/
theorem filterIn_from_ys {α : Type} [BEq α] [LawfulBEq α] (ys xs : List α) :
    ∀ x ∈ filterIn ys xs, x ∈ ys := by
  intro x hx
  simp only [filterIn, List.mem_filter] at hx
  exact hx.1

/-- Characterization of `filterIn` membership. -/
theorem mem_filterIn_iff {α : Type} [BEq α] [LawfulBEq α] (ys xs : List α) (x : α) :
    x ∈ filterIn ys xs ↔ x ∈ ys ∧ x ∈ xs := by
  simp only [filterIn, List.mem_filter, List.elem_iff]

/-! ## List to NonEmptyHashSet Conversion -/

/-- Convert a non-empty list to a NonEmptyHashSet. -/
def listToNonEmpty {α : Type} [BEq α] [Hashable α] : (xs : List α) → xs ≠ [] → NonEmptyHashSet α
  | x :: xs, _ => ⟨x, Std.HashSet.ofList xs⟩

/-- `listToNonEmpty` preserves membership. -/
theorem listToNonEmpty_toList {α : Type} [BEq α] [Hashable α] [LawfulBEq α]
    (xs : List α) (h : xs ≠ []) (x : α) :
    x ∈ (listToNonEmpty xs h).toList ↔ x ∈ xs := by
  cases xs with
  | nil => contradiction
  | cons y ys =>
    simp only [listToNonEmpty, NonEmptyHashSet.toList, List.mem_cons]
    constructor
    · intro hx
      cases hx with
      | inl h => left; exact h
      | inr h =>
        right
        exact List.elem_iff.mp (Std.HashSet.mem_ofList.mp (Std.HashSet.mem_toList.mp h))
    · intro hx
      cases hx with
      | inl h => left; exact h
      | inr h =>
        right
        exact Std.HashSet.mem_toList.mpr (Std.HashSet.mem_ofList.mpr (List.elem_iff.mpr h))

/-! ## Powerset Completeness

The key theorem: every non-empty subset of a list appears in its powerset
(up to `sameElements` equivalence).
-/

/-- **Completeness**: If all elements of `s` are in `xs`, then `s` has a
    `sameElements`-equivalent representative in `nonEmptyPowerset xs`. -/
theorem nonEmptyPowerset_complete {α : Type} [BEq α] [Hashable α] [LawfulBEq α]
    (xs : List α) (s : NonEmptyHashSet α) :
    (∀ x ∈ s.toList, x ∈ xs) →
    ∃ s' ∈ nonEmptyPowerset xs, s.sameElements s' = true := by
  induction xs generalizing s with
  | nil =>
    intro h
    have := h s.head (List.Mem.head _)
    contradiction
  | cons y ys ih =>
    intro h_subset
    by_cases hy : y ∈ s.toList
    · -- Case: y is in s
      let rest_elems := filterIn s.toList ys
      by_cases h_rest_empty : rest_elems = []
      · -- Subcase: s contains only y
        refine ⟨⟨y, {}⟩, ?_, ?_⟩
        · unfold nonEmptyPowerset; apply List.mem_cons_self
        · simp only [NonEmptyHashSet.sameElements, Bool.and_eq_true, List.all_eq_true]
          constructor
          · intro x hx
            rw [singleton_contains]
            have hx_in_xs := h_subset x hx
            simp only [List.mem_cons] at hx_in_xs
            cases hx_in_xs with
            | inl h => rw [h]; exact beq_self_eq_true _
            | inr h =>
              have : x ∈ rest_elems := mem_filterIn_iff s.toList ys x |>.mpr ⟨hx, h⟩
              rw [h_rest_empty] at this; contradiction
          · intro x hx
            rw [singleton_toList] at hx
            simp only [List.mem_singleton] at hx
            rw [hx]; exact NonEmptyHashSet.toList_mem_contains s y hy
      · -- Subcase: s contains y and elements from ys
        have h_rest_ne : rest_elems ≠ [] := h_rest_empty
        let s_rest := listToNonEmpty rest_elems h_rest_ne
        have h_rest_in_ys : ∀ x ∈ s_rest.toList, x ∈ ys := by
          intro x hx
          rw [listToNonEmpty_toList] at hx
          exact filterIn_subset s.toList ys x hx
        obtain ⟨s', hs'_in_pow, hs'_same⟩ := ih s_rest h_rest_in_ys
        refine ⟨⟨y, s'.rest.insert s'.head⟩, withX_in_powerset y ys s' hs'_in_pow, ?_⟩
        simp only [NonEmptyHashSet.sameElements, Bool.and_eq_true, List.all_eq_true]
        constructor
        · intro x hx
          have h_mem := withX_toList_mem y s' x
          have hx_in_xs := h_subset x hx
          simp only [List.mem_cons] at hx_in_xs
          cases hx_in_xs with
          | inl h =>
            have : x ∈ (⟨y, s'.rest.insert s'.head⟩ : NonEmptyHashSet α).toList := h_mem.mpr (Or.inl h)
            exact NonEmptyHashSet.toList_mem_contains _ x this
          | inr h =>
            have hx_in_rest : x ∈ rest_elems := mem_filterIn_iff s.toList ys x |>.mpr ⟨hx, h⟩
            have hx_in_s_rest : x ∈ s_rest.toList := listToNonEmpty_toList rest_elems h_rest_ne x |>.mpr hx_in_rest
            simp only [NonEmptyHashSet.sameElements, Bool.and_eq_true, List.all_eq_true] at hs'_same
            have hx_in_s' := NonEmptyHashSet.all_mem_toList s' x (hs'_same.1 x hx_in_s_rest)
            have : x ∈ (⟨y, s'.rest.insert s'.head⟩ : NonEmptyHashSet α).toList := h_mem.mpr (Or.inr hx_in_s')
            exact NonEmptyHashSet.toList_mem_contains _ x this
        · intro x hx
          have h_mem := withX_toList_mem y s' x
          have hx' := h_mem.mp hx
          cases hx' with
          | inl h => rw [h]; exact NonEmptyHashSet.toList_mem_contains s y hy
          | inr h =>
            simp only [NonEmptyHashSet.sameElements, Bool.and_eq_true, List.all_eq_true] at hs'_same
            have hx_in_s_rest := NonEmptyHashSet.all_mem_toList s_rest x (hs'_same.2 x h)
            rw [listToNonEmpty_toList] at hx_in_s_rest
            have hx_in_s := filterIn_from_ys s.toList ys x hx_in_s_rest
            exact NonEmptyHashSet.toList_mem_contains s x hx_in_s
    · -- Case: y is not in s, so all elements are in ys
      have h_in_ys : ∀ x ∈ s.toList, x ∈ ys := by
        intro x hx
        have := h_subset x hx
        simp only [List.mem_cons] at this
        cases this with
        | inl h => exfalso; rw [h] at hx; exact hy hx
        | inr h => exact h
      obtain ⟨s', hs'_in_pow, hs'_same⟩ := ih s h_in_ys
      refine ⟨s', ?_, hs'_same⟩
      unfold nonEmptyPowerset
      apply List.mem_cons_of_mem
      apply List.mem_append_right
      exact hs'_in_pow

/-! ## SameElements and SubsetOf Bridge

These lemmas connect `sameElements` (structural equality up to representation)
with `subsetOf` (containment in event tags).
-/

/-- If query tags are subset of event tags, find a matching powerset element. -/
theorem sameElements_exists_in_powerset (tags : NonEmptyHashSet Tag) (eventTags : Std.HashSet Tag)
    (h : tags.subsetOf eventTags = true) :
    ∃ s ∈ nonEmptyPowerset eventTags.toList, tags.sameElements s = true := by
  apply nonEmptyPowerset_complete
  intro x hx
  simp only [NonEmptyHashSet.subsetOf, Bool.and_eq_true] at h
  simp only [NonEmptyHashSet.toList, List.mem_cons] at hx
  cases hx with
  | inl h' =>
    rw [h']
    exact Std.HashSet.mem_toList.mpr (Std.HashSet.mem_iff_contains.mpr h.1)
  | inr h' =>
    have : x ∈ tags.rest.toList := h'
    have hx_in_rest : tags.rest.contains x := Std.HashSet.mem_iff_contains.mp (Std.HashSet.mem_toList.mp this)
    have : tags.rest.all eventTags.contains = true := h.2
    have hx_in_event : eventTags.contains x := Std.HashSet.all_eq_true_iff_forall_mem.mp this x (Std.HashSet.mem_iff_contains.mpr hx_in_rest)
    exact Std.HashSet.mem_toList.mpr (Std.HashSet.mem_iff_contains.mpr hx_in_event)

/-- `sameElements` plus subset implies `subsetOf`. -/
theorem sameElements_subsetOf (tags s : NonEmptyHashSet Tag) (eventTags : Std.HashSet Tag)
    (hsame : tags.sameElements s = true)
    (hs_subset : ∀ x ∈ s.toList, x ∈ eventTags.toList) :
    tags.subsetOf eventTags = true := by
  simp only [NonEmptyHashSet.subsetOf, Bool.and_eq_true]
  simp only [NonEmptyHashSet.sameElements, Bool.and_eq_true] at hsame
  constructor
  · -- eventTags.contains tags.head
    have h1 : tags.toList.all (s.contains ·) = true := hsame.1
    have h_head : s.contains tags.head := by
      have := List.all_eq_true.mp h1 tags.head (List.Mem.head _)
      exact this
    have h_head_in_s := NonEmptyHashSet.all_mem_toList s tags.head h_head
    have h_head_in_event := hs_subset tags.head h_head_in_s
    exact Std.HashSet.mem_iff_contains.mp (Std.HashSet.mem_toList.mp h_head_in_event)
  · -- tags.rest.all eventTags.contains
    apply Std.HashSet.all_eq_true_iff_forall_mem.mpr
    intro x hx
    have h1 : tags.toList.all (s.contains ·) = true := hsame.1
    have h_x_in_tags : x ∈ tags.toList := List.Mem.tail _ (Std.HashSet.mem_toList.mpr hx)
    have h_x_in_s : s.contains x := List.all_eq_true.mp h1 x h_x_in_tags
    have h_x_in_s_list := NonEmptyHashSet.all_mem_toList s x h_x_in_s
    have h_x_in_event := hs_subset x h_x_in_s_list
    exact Std.HashSet.mem_iff_contains.mp (Std.HashSet.mem_toList.mp h_x_in_event)

/-! ## Bucket Extraction Lemmas

Lemmas for extracting bucket information from membership proofs.
-/

/-- A bucket in the tag bucket HashSet must be a tagBucket with a powerset element. -/
theorem tagBucket_of_mem_ofList (e : Event) (wb : Bucket)
    (h : wb ∈ (Std.HashSet.ofList ((nonEmptyPowerset e.tags.toList).map (Bucket.tagBucket e.type))).toList) :
    ∃ s ∈ nonEmptyPowerset e.tags.toList, wb = Bucket.tagBucket e.type s := by
  have h1 := Std.HashSet.mem_toList.mp h
  have h2 := Std.HashSet.mem_ofList.mp h1
  have h3 := List.elem_iff.mp h2
  simp only [List.mem_map] at h3
  obtain ⟨s, hs, rfl⟩ := h3
  exact ⟨s, hs, rfl⟩

/-- TagBucket constructor is injective in the type component. -/
theorem tagBucket_type_eq (t : EventType) (s s' : NonEmptyHashSet Tag) (etype : EventType)
    (h : Bucket.tagBucket t s = Bucket.tagBucket etype s') : t = etype := by
  injection h

end DCBConflict
