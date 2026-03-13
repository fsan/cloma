MODEL ?= glm-5:cloud
WORKSPACE ?= $(PWD)
OLLAMA_PORT ?= 11434
FLAGS ?=

.DEFAULT_GOAL := run
.PHONY: setup run doctor shell logs stop clean template template-clean

setup:
	CLAUDE_CODE_MODEL="$(MODEL)" OLLAMA_PORT="$(OLLAMA_PORT)" ./scripts/setup.sh "$(WORKSPACE)"

run:
	CLAUDE_CODE_MODEL="$(MODEL)" OLLAMA_PORT="$(OLLAMA_PORT)" CLAUDE_CODE_FLAGS="$(FLAGS)" ./scripts/run-claude-code.sh "$(WORKSPACE)"

doctor:
	CLAUDE_CODE_MODEL="$(MODEL)" OLLAMA_PORT="$(OLLAMA_PORT)" ./scripts/doctor.sh "$(WORKSPACE)"

shell:
	./scripts/shell.sh "$(WORKSPACE)"

logs:
	./scripts/logs.sh "$(WORKSPACE)"

stop:
	./scripts/stop-sandbox.sh "$(WORKSPACE)"

clean:
	./scripts/clean-sandbox.sh "$(WORKSPACE)"

template:
	./scripts/bake-template.sh

template-clean:
	./scripts/clean-template.sh