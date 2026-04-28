BIN     := termask
INSTALL := $(HOME)/.local/bin

.PHONY: build install shell-zsh shell-bash clean

build:
	go build -o $(BIN) ./cmd/termask

install: build
	mkdir -p $(INSTALL)
	cp $(BIN) $(INSTALL)/$(BIN)
	@echo "✓ Installed to $(INSTALL)/$(BIN)"
	@echo ""
	@echo "Add the shell integration to your .zshrc:"
	@echo "  $(INSTALL)/$(BIN) shell --shell zsh >> ~/.zshrc && source ~/.zshrc"
	@echo ""
	@echo "Or for bash:"
	@echo "  $(INSTALL)/$(BIN) shell --shell bash >> ~/.bashrc && source ~/.bashrc"

shell-zsh:
	@$(INSTALL)/$(BIN) shell --shell zsh

shell-bash:
	@$(INSTALL)/$(BIN) shell --shell bash

clean:
	rm -f $(BIN)
