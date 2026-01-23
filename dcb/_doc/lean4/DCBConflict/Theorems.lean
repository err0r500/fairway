import DCBConflict.Model
import DCBConflict.Matching
import DCBConflict.Operations
import DCBConflict.Lemmas

namespace DCBConflict

/-- Completeness: if an event matches a query, conflict is detected.
    Forward direction of the main theorem (no false negatives). -/
theorem completeness (e : Event) (q : Query) (h : matchesQuery e q) : conflictDetected e q := by
  obtain ⟨hversion, item, hitem_mem, hitem_match⟩ := h
  simp only [conflictDetected]
  cases item with
  | typeOnly types =>
    simp only [matchesItem] at hitem_match
    have htype_mem := NonEmptyList.contains_mem_toList types e.type hitem_match
    refine ⟨Bucket.typeBucket e.type, typeBucket_mem_writeBuckets e, ?_⟩
    refine ⟨⟨Bucket.typeBucket e.type, q.afterVersion⟩, ?_, rfl, hversion⟩
    simp only [readTargets, List.mem_flatMap]
    refine ⟨QueryItem.typeOnly types, hitem_mem, ?_⟩
    simp only [readTargetsItem, List.mem_map]
    exact ⟨e.type, htype_mem, rfl⟩

  | tagsOnly tags =>
    simp only [matchesItem] at hitem_match
    have htags_subset : ∀ x ∈ tags.toList, x ∈ e.tags := by
      intro x hx
      simp only [NonEmptyList.subsetOf, Bool.and_eq_true] at hitem_match
      simp only [NonEmptyList.toList, List.mem_cons] at hx
      cases hx with
      | inl h => exact h ▸ List.elem_iff.mp hitem_match.1
      | inr h => exact List.elem_iff.mp (List.all_eq_true.mp hitem_match.2 x h)
    obtain ⟨s, hs_pow, hs_same⟩ := subset_in_powerset e.tags tags htags_subset
    have hwb : Bucket.tagBucket e.type s ∈ (writeBuckets e).toList := by
      simp only [writeBuckets, NonEmptyList.toList, List.mem_cons, List.mem_map]
      right; exact ⟨s, hs_pow, rfl⟩
    refine ⟨Bucket.tagBucket e.type s, hwb, ?_⟩
    refine ⟨⟨Bucket.tagBucket e.type s, q.afterVersion⟩, ?_, rfl, hversion⟩
    simp only [readTargets, List.mem_flatMap]
    refine ⟨QueryItem.tagsOnly tags, hitem_mem, ?_⟩
    simp only [readTargetsItem, List.mem_map, List.mem_filter]
    exact ⟨s, ⟨hs_pow, hs_same⟩, rfl⟩

  | typesAndTags types tags =>
    simp only [matchesItem, Bool.and_eq_true] at hitem_match
    have htypes := hitem_match.1
    have htags := hitem_match.2
    have htype_mem := NonEmptyList.contains_mem_toList types e.type htypes
    have htags_subset : ∀ x ∈ tags.toList, x ∈ e.tags := by
      intro x hx
      simp only [NonEmptyList.subsetOf, Bool.and_eq_true] at htags
      simp only [NonEmptyList.toList, List.mem_cons] at hx
      cases hx with
      | inl h => exact h ▸ List.elem_iff.mp htags.1
      | inr h => exact List.elem_iff.mp (List.all_eq_true.mp htags.2 x h)
    obtain ⟨s, hs_pow, hs_same⟩ := subset_in_powerset e.tags tags htags_subset
    have hwb : Bucket.tagBucket e.type s ∈ (writeBuckets e).toList := by
      simp only [writeBuckets, NonEmptyList.toList, List.mem_cons, List.mem_map]
      right; exact ⟨s, hs_pow, rfl⟩
    refine ⟨Bucket.tagBucket e.type s, hwb, ?_⟩
    refine ⟨⟨Bucket.tagBucket e.type s, q.afterVersion⟩, ?_, rfl, hversion⟩
    simp only [readTargets, List.mem_flatMap]
    refine ⟨QueryItem.typesAndTags types tags, hitem_mem, ?_⟩
    simp only [readTargetsItem, List.mem_flatMap, List.mem_filter, List.mem_map]
    exact ⟨s, ⟨hs_pow, hs_same⟩, e.type, htype_mem, rfl⟩

/-- Precision: if conflict is detected, the event matches the query.
    Backward direction of the main theorem (no false positives). -/
theorem precision (e : Event) (q : Query) (h : conflictDetected e q) : matchesQuery e q := by
  obtain ⟨wb, hwb_mem, rt, hrt_mem, hwb_eq, hversion⟩ := h
  simp only [matchesQuery]
  simp only [readTargets, List.mem_flatMap] at hrt_mem
  obtain ⟨item, hitem_mem, hrt_item⟩ := hrt_mem
  -- First show version condition
  have hv : e.version > q.afterVersion := by
    cases item <;> simp only [readTargetsItem, List.mem_map, List.mem_filter, List.mem_flatMap] at hrt_item
    · obtain ⟨_, _, rfl⟩ := hrt_item; exact hversion
    · obtain ⟨_, ⟨_, _⟩, rfl⟩ := hrt_item; exact hversion
    · obtain ⟨_, ⟨_, _⟩, _, _, rfl⟩ := hrt_item; exact hversion
  refine ⟨hv, item, hitem_mem, ?_⟩
  cases item with
  | typeOnly types =>
    simp only [readTargetsItem, List.mem_map] at hrt_item
    obtain ⟨t, ht_mem, hrt_eq⟩ := hrt_item
    simp only [writeBuckets, NonEmptyList.toList, List.mem_cons, List.mem_map] at hwb_mem
    simp only [matchesItem]
    rcases hwb_mem with rfl | ⟨s, _, rfl⟩
    · -- wb = typeBucket e.type, rt.bucket = typeBucket t
      simp only [← hrt_eq] at hwb_eq
      simp only [Bucket.typeBucket.injEq] at hwb_eq
      rw [hwb_eq]
      exact NonEmptyList.mem_toList_contains types t ht_mem
    · -- wb = tagBucket, rt.bucket = typeBucket t - contradiction
      simp only [← hrt_eq] at hwb_eq
      exact absurd hwb_eq (by simp)

  | tagsOnly tags =>
    simp only [readTargetsItem, List.mem_map, List.mem_filter] at hrt_item
    obtain ⟨s, ⟨hs_pow, hs_same⟩, hrt_eq⟩ := hrt_item
    simp only [writeBuckets, NonEmptyList.toList, List.mem_cons, List.mem_map] at hwb_mem
    simp only [matchesItem]
    rcases hwb_mem with rfl | ⟨s', hs'_pow, rfl⟩
    · -- wb = typeBucket, rt.bucket = tagBucket - contradiction
      simp only [← hrt_eq] at hwb_eq
      exact absurd hwb_eq (by simp)
    · -- wb = tagBucket e.type s', rt.bucket = tagBucket e.type s
      simp only [← hrt_eq, Bucket.tagBucket.injEq] at hwb_eq
      obtain ⟨_, hs_eq⟩ := hwb_eq
      have hs'_eq_s : s' = s := eq_of_beq (hs_eq ▸ NonEmptyList.beq_rfl s)
      rw [hs'_eq_s] at hs'_pow
      exact sameElements_powerset_subsetOf tags s e.tags hs_pow hs_same

  | typesAndTags types tags =>
    simp only [readTargetsItem, List.mem_flatMap, List.mem_filter, List.mem_map] at hrt_item
    obtain ⟨s, ⟨hs_pow, hs_same⟩, t, ht_mem, hrt_eq⟩ := hrt_item
    simp only [writeBuckets, NonEmptyList.toList, List.mem_cons, List.mem_map] at hwb_mem
    simp only [matchesItem, Bool.and_eq_true]
    rcases hwb_mem with rfl | ⟨s', hs'_pow, rfl⟩
    · -- wb = typeBucket, rt.bucket = tagBucket t s - contradiction
      simp only [← hrt_eq] at hwb_eq
      exact absurd hwb_eq (by simp)
    · -- wb = tagBucket e.type s', rt.bucket = tagBucket t s
      simp only [← hrt_eq, Bucket.tagBucket.injEq] at hwb_eq
      obtain ⟨htype_eq, hs_eq⟩ := hwb_eq
      have hs'_eq_s : s' = s := eq_of_beq (hs_eq ▸ NonEmptyList.beq_rfl s)
      rw [hs'_eq_s] at hs'_pow
      constructor
      · rw [htype_eq]
        exact NonEmptyList.mem_toList_contains types t ht_mem
      · exact sameElements_powerset_subsetOf tags s e.tags hs_pow hs_same

/-- Main theorem: matchesQuery and conflictDetected are equivalent.
    Proves the DCB conflict detection mechanism is both complete and precise. -/
theorem conflict_iff_matches (e : Event) (q : Query) : matchesQuery e q ↔ conflictDetected e q :=
  ⟨completeness e q, precision e q⟩

end DCBConflict
