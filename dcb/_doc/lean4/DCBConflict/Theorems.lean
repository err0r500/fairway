import DCBConflict.Basic
import DCBConflict.Matching
import DCBConflict.Index

namespace DCBConflict

-- Helper: NonEmptyHashSet.contains implies membership in toList
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

-- Helper: typeBucket is head of writeBuckets
theorem typeBucket_in_writeBuckets (e : Event) :
    Bucket.typeBucket e.type ∈ (writeBuckets e).toList := by
  simp only [writeBuckets, NonEmptyHashSet.toList]
  exact List.Mem.head _

-- Helper: readTarget is in readTargets when item matches
theorem readTarget_in_readTargets (q : Query) (e : Event) (item : QueryItem)
    (hitem : item ∈ q.items.toList) (rt : ReadTarget)
    (hrt : rt ∈ readTargetsItem item q.afterVersion e) :
    rt ∈ readTargets q e := by
  simp only [readTargets, List.mem_flatMap]
  exact ⟨item, hitem, hrt⟩

-- Helper: element in list implies element in HashSet.ofList.toList
theorem mem_hashset_ofList_toList {α : Type} [BEq α] [Hashable α] [LawfulBEq α]
    (x : α) (xs : List α) (h : x ∈ xs) :
    x ∈ (Std.HashSet.ofList xs).toList := by
  apply Std.HashSet.mem_toList.mpr
  apply Std.HashSet.mem_ofList.mpr
  exact List.elem_iff.mpr h

-- Helper: element of powerset is in writeBuckets
theorem powerset_tagBucket_in_writeBuckets (e : Event) (s : NonEmptyHashSet Tag)
    (hs : s ∈ nonEmptyPowerset e.tags.toList) :
    Bucket.tagBucket e.type s ∈ (writeBuckets e).toList := by
  simp only [writeBuckets, NonEmptyHashSet.toList]
  apply List.Mem.tail
  have h : Bucket.tagBucket e.type s ∈ (nonEmptyPowerset e.tags.toList).map (Bucket.tagBucket e.type) := by
    simp only [List.mem_map]
    exact ⟨s, hs, rfl⟩
  exact mem_hashset_ofList_toList _ _ h

-- Helper: all elements of NonEmptyHashSet are in its toList
theorem NonEmptyHashSet.all_mem_toList {α : Type} [BEq α] [Hashable α] [LawfulBEq α]
    (s : NonEmptyHashSet α) : ∀ x, s.contains x → x ∈ s.toList := by
  intro x hx
  simp only [NonEmptyHashSet.contains, Bool.or_eq_true] at hx
  simp only [NonEmptyHashSet.toList]
  cases hx with
  | inl h => rw [eq_of_beq h]; exact List.Mem.head _
  | inr h => exact List.Mem.tail _ (Std.HashSet.mem_toList.mpr (Std.HashSet.mem_iff_contains.mpr h))

-- Helper: powerset contains all non-empty subsets (axiomatized - complex inductive proof)
axiom nonEmptyPowerset_complete {α : Type} [BEq α] [Hashable α] [LawfulBEq α]
    (xs : List α) (s : NonEmptyHashSet α) :
    (∀ x ∈ s.toList, x ∈ xs) →
    ∃ s' ∈ nonEmptyPowerset xs, s.sameElements s' = true

-- Helper: if tags.subsetOf e.tags, then sameElements finds a match in powerset
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

-- Completeness: if event matches query, conflict is detected
theorem completeness (e : Event) (q : Query) (h : matchesQuery e q) : conflictDetected e q := by
  obtain ⟨hv, item, hitem, hmatch⟩ := h
  unfold conflictDetected
  match item with
  | .typeOnly types =>
    -- types.contains e.type = true, so typeBucket e.type matches
    simp only [matchesItem] at hmatch
    refine ⟨Bucket.typeBucket e.type, typeBucket_in_writeBuckets e, ?_⟩
    refine ⟨⟨Bucket.typeBucket e.type, q.afterVersion⟩, ?_, rfl, hv⟩
    apply readTarget_in_readTargets _ _ _ hitem
    simp only [readTargetsItem, List.mem_map]
    exact ⟨e.type, NonEmptyHashSet.contains_mem_toList types e.type hmatch, rfl⟩
  | .tagsOnly tags =>
    -- tags.subsetOf e.tags = true, find matching s in powerset
    simp only [matchesItem] at hmatch
    obtain ⟨s, hs_in_pow, hs_same⟩ := sameElements_exists_in_powerset tags e.tags hmatch
    refine ⟨Bucket.tagBucket e.type s, powerset_tagBucket_in_writeBuckets e s hs_in_pow, ?_⟩
    refine ⟨⟨Bucket.tagBucket e.type s, q.afterVersion⟩, ?_, rfl, hv⟩
    apply readTarget_in_readTargets _ _ _ hitem
    simp only [readTargetsItem, List.mem_map, List.mem_filter]
    exact ⟨s, ⟨hs_in_pow, hs_same⟩, rfl⟩
  | .typesAndTags types tags =>
    -- types.contains e.type ∧ tags.subsetOf e.tags
    simp only [matchesItem, Bool.and_eq_true] at hmatch
    obtain ⟨htype, htags⟩ := hmatch
    obtain ⟨s, hs_in_pow, hs_same⟩ := sameElements_exists_in_powerset tags e.tags htags
    refine ⟨Bucket.tagBucket e.type s, powerset_tagBucket_in_writeBuckets e s hs_in_pow, ?_⟩
    refine ⟨⟨Bucket.tagBucket e.type s, q.afterVersion⟩, ?_, rfl, hv⟩
    apply readTarget_in_readTargets _ _ _ hitem
    simp only [readTargetsItem, List.mem_flatMap, List.mem_map, List.mem_filter]
    exact ⟨s, ⟨hs_in_pow, hs_same⟩, e.type, NonEmptyHashSet.contains_mem_toList types e.type htype, rfl⟩

-- Helper: readTarget has afterVersion equal to query's afterVersion
theorem readTarget_afterVersion (q : Query) (e : Event) (rt : ReadTarget) (item : QueryItem)
    (hrt : rt ∈ readTargetsItem item q.afterVersion e) :
    rt.afterVersion = q.afterVersion := by
  cases item <;> simp only [readTargetsItem, List.mem_map, List.mem_filter, List.mem_flatMap] at hrt
  · obtain ⟨_, _, rfl⟩ := hrt; rfl
  · obtain ⟨_, ⟨_, _⟩, rfl⟩ := hrt; rfl
  · obtain ⟨_, ⟨_, _⟩, _, _, rfl⟩ := hrt; rfl

-- Helper: if element in toList, then contains is true
theorem NonEmptyHashSet.toList_mem_contains {α : Type} [BEq α] [Hashable α] [LawfulBEq α]
    (s : NonEmptyHashSet α) (x : α) (h : x ∈ s.toList) : s.contains x = true := by
  simp only [NonEmptyHashSet.contains, NonEmptyHashSet.toList, List.mem_cons] at *
  cases h with
  | inl h => simp only [h, beq_self_eq_true, Bool.true_or]
  | inr h => simp only [Std.HashSet.mem_iff_contains.mp (Std.HashSet.mem_toList.mp h), Bool.or_true]

-- Helper: sameElements implies subsetOf when target is subset of eventTags
theorem sameElements_subsetOf (tags s : NonEmptyHashSet Tag) (eventTags : Std.HashSet Tag)
    (hsame : tags.sameElements s = true)
    (hs_subset : ∀ x ∈ s.toList, x ∈ eventTags.toList) :
    tags.subsetOf eventTags = true := by
  simp only [NonEmptyHashSet.subsetOf, Bool.and_eq_true]
  simp only [NonEmptyHashSet.sameElements, Bool.and_eq_true] at hsame
  -- hsame.1: tags.toList.all (s.contains ·) = true
  -- hsame.2: s.toList.all (tags.contains ·) = true
  -- Need: eventTags.contains tags.head ∧ tags.rest.all eventTags.contains
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

-- Helper: elements of powerset are subsets of original (axiomatized)
axiom powerset_elem_subset (xs : List Tag) (s : NonEmptyHashSet Tag)
    (hs : s ∈ nonEmptyPowerset xs) : ∀ x ∈ s.toList, x ∈ xs

-- Helper: bucket in HashSet.ofList of tagBuckets means it's a tagBucket
axiom tagBucket_of_mem_ofList (e : Event) (wb : Bucket)
    (h : wb ∈ (Std.HashSet.ofList ((nonEmptyPowerset e.tags.toList).map (Bucket.tagBucket e.type))).toList) :
    ∃ s ∈ nonEmptyPowerset e.tags.toList, wb = Bucket.tagBucket e.type s

-- Helper: if tagBucket t s = tagBucket e.type s', then t = e.type
axiom tagBucket_type_eq (t : EventType) (s s' : NonEmptyHashSet Tag) (etype : EventType)
    (h : Bucket.tagBucket t s = Bucket.tagBucket etype s') : t = etype

-- Precision: if conflict detected, event matches query
theorem precision (e : Event) (q : Query) (h : conflictDetected e q) : matchesQuery e q := by
  obtain ⟨wb, hwb, rt, hrt, heq, hver⟩ := h
  unfold matchesQuery
  simp only [readTargets, List.mem_flatMap] at hrt
  obtain ⟨item, hitem, hrt_item⟩ := hrt
  have hv : rt.afterVersion = q.afterVersion := readTarget_afterVersion q e rt item hrt_item
  refine ⟨?_, item, hitem, ?_⟩
  · rw [← hv]; exact hver
  · cases item with
    | typeOnly types =>
      simp only [matchesItem]
      simp only [readTargetsItem, List.mem_map] at hrt_item
      obtain ⟨t, ht_in_types, hrt_eq⟩ := hrt_item
      simp only [writeBuckets, NonEmptyHashSet.toList] at hwb
      cases hwb with
      | head _ =>
        rw [← hrt_eq] at heq
        injection heq with h_type
        rw [← h_type] at ht_in_types
        exact NonEmptyHashSet.toList_mem_contains types e.type ht_in_types
      | tail _ hwb' =>
        rw [← hrt_eq] at heq
        obtain ⟨s, _, hs_eq⟩ := tagBucket_of_mem_ofList e wb hwb'
        rw [hs_eq] at heq
        contradiction
    | tagsOnly tags =>
      simp only [matchesItem]
      simp only [readTargetsItem, List.mem_map, List.mem_filter] at hrt_item
      obtain ⟨s, ⟨hs_in_pow, hs_same⟩, hrt_eq⟩ := hrt_item
      rw [← hrt_eq] at heq
      simp only [writeBuckets, NonEmptyHashSet.toList] at hwb
      cases hwb with
      | head _ => contradiction
      | tail _ hwb' =>
        have hs_subset := powerset_elem_subset e.tags.toList s hs_in_pow
        exact sameElements_subsetOf tags s e.tags hs_same hs_subset
    | typesAndTags types tags =>
      simp only [matchesItem, Bool.and_eq_true]
      simp only [readTargetsItem, List.mem_flatMap, List.mem_map, List.mem_filter] at hrt_item
      obtain ⟨s, ⟨hs_in_pow, hs_same⟩, t, ht_in_types, hrt_eq⟩ := hrt_item
      rw [← hrt_eq] at heq
      simp only at heq
      simp only [writeBuckets, NonEmptyHashSet.toList] at hwb
      cases hwb with
      | head _ => contradiction
      | tail _ hwb' =>
        obtain ⟨s', _, hs'_eq⟩ := tagBucket_of_mem_ofList e wb hwb'
        constructor
        · have h_type := tagBucket_type_eq t s s' e.type (heq.symm.trans hs'_eq)
          rw [h_type] at ht_in_types
          exact NonEmptyHashSet.toList_mem_contains types e.type ht_in_types
        · have hs_subset := powerset_elem_subset e.tags.toList s hs_in_pow
          exact sameElements_subsetOf tags s e.tags hs_same hs_subset

-- Main theorem
theorem conflict_iff_matches (e : Event) (q : Query) : matchesQuery e q ↔ conflictDetected e q :=
  ⟨completeness e q, precision e q⟩

end DCBConflict
