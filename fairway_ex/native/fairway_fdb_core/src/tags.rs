/// Tag subset generation for the tag tree index.
///
/// Mirrors Go's generateAllSubsets in dcb/dcb.go.
/// For tags [c, a, b] (sorted: [a, b, c]), generates all non-empty subsets
/// in bit-mask order:
///   [a], [b], [a, b], [c], [a, c], [b, c], [a, b, c]

/// Generate all non-empty subsets of the given tags.
/// Tags are sorted alphabetically before subset generation (normalization).
/// Returns subsets as Vec<Vec<String>>, each subset sorted alphabetically.
pub fn generate_all_subsets(tags: &[String]) -> Vec<Vec<String>> {
    if tags.is_empty() {
        return vec![];
    }

    let mut sorted_tags = tags.to_vec();
    sorted_tags.sort();

    let n = sorted_tags.len();
    let total = (1usize << n) - 1; // 2^n - 1 (exclude empty set)
    let mut result = Vec::with_capacity(total);

    for mask in 1..=total {
        let subset: Vec<String> = (0..n)
            .filter(|&i| mask & (1 << i) != 0)
            .map(|i| sorted_tags[i].clone())
            .collect();
        result.push(subset);
    }

    result
}

/// Sort a slice of tag strings alphabetically (in-place).
pub fn sort_tags(tags: &mut Vec<String>) {
    tags.sort();
}

/// Return a sorted copy of the given tag strings.
pub fn sorted_tags(tags: &[String]) -> Vec<String> {
    let mut t = tags.to_vec();
    t.sort();
    t
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_empty_tags() {
        assert!(generate_all_subsets(&[]).is_empty());
    }

    #[test]
    fn test_single_tag() {
        let subsets = generate_all_subsets(&["a".to_string()]);
        assert_eq!(subsets, vec![vec!["a".to_string()]]);
    }

    #[test]
    fn test_two_tags() {
        let subsets = generate_all_subsets(&["b".to_string(), "a".to_string()]);
        // sorted input = [a, b]
        // mask 1 (0b01) = [a]
        // mask 2 (0b10) = [b]
        // mask 3 (0b11) = [a, b]
        assert_eq!(
            subsets,
            vec![
                vec!["a".to_string()],
                vec!["b".to_string()],
                vec!["a".to_string(), "b".to_string()],
            ]
        );
    }

    #[test]
    fn test_three_tags() {
        let subsets = generate_all_subsets(&["c".to_string(), "a".to_string(), "b".to_string()]);
        // sorted = [a, b, c]; 7 subsets
        assert_eq!(subsets.len(), 7);
        // First is [a], last is [a, b, c]
        assert_eq!(subsets[0], vec!["a".to_string()]);
        assert_eq!(subsets[6], vec!["a".to_string(), "b".to_string(), "c".to_string()]);
    }
}
