import Lake
open Lake DSL

package DCBConflict where
  leanOptions := #[
    ⟨`autoImplicit, false⟩,
    ⟨`linter.unusedVariables, true⟩
  ]

require batteries from git "https://github.com/leanprover-community/batteries" @ "main"

@[default_target]
lean_lib DCBConflict where
  globs := #[.submodules `DCBConflict]
