import DCBConflict.Model
import DCBConflict.Matching
import DCBConflict.Operations
import DCBConflict.Lemmas


namespace DCBConflict

-- Completeness: if event matches query -> conflict is detected
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

-- Precision: if conflict detected -> event matches query
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
