RUST_DIR      := fairway_ex/native
RUST_TARGET   := $(RUST_DIR)/target/release
LIB_NAME      := fairway_fdb_c
HEADER_DIR    := $(RUST_DIR)/fairway_fdb_c/include

.PHONY: rust-lib rust-lib-debug clean-rust cstore-env

## Build the Rust fairway_fdb_c shared library (release mode).
rust-lib:
	cd $(RUST_DIR) && cargo build --release -p $(LIB_NAME)

## Build the Rust fairway_fdb_c shared library (debug mode, faster compile).
rust-lib-debug:
	cd $(RUST_DIR) && cargo build -p $(LIB_NAME)

## Print the shell exports needed before `go build ./dcb/cstore/...`
cstore-env:
	@echo "export CGO_CFLAGS=\"-I$$(pwd)/$(HEADER_DIR)\""
	@echo "export CGO_LDFLAGS=\"-L$$(pwd)/$(RUST_TARGET) -l$(LIB_NAME)\""
	@echo "export LD_LIBRARY_PATH=\"$$(pwd)/$(RUST_TARGET):\$$LD_LIBRARY_PATH\""

## Clean Rust build artifacts.
clean-rust:
	cd $(RUST_DIR) && cargo clean
