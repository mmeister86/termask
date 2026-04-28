# termask — BYOK Terminal AI Assistant

Frag KI-Modelle direkt im Terminal — mit deinen eigenen API-Keys.
Anthropic ist als Standard voreingestellt, alle anderen Provider sind optional.

```
$ # Leere Zeile + Ctrl+K
termask › schreibe ein shell script mit ffmpeg das alle mkv zu mp4 konvertiert

┌────────────────────────────────────────────────────────────┐
│ [anthropic / claude-sonnet-4-6]                             │
├────────────────────────────────────────────────────────────┤
│ schreibe ein shell script mit ffmpeg das alle mkv zu mp4   │
│ konvertiert                                                 │
└────────────────────────────────────────────────────────────┘

```bash
#!/usr/bin/env bash
set -euo pipefail
for f in *.mkv; do
  [[ -f "$f" ]] || continue
  ffmpeg -i "$f" -codec copy "${f%.mkv}.mp4"
done
```
```

## Installation

```bash
git clone https://github.com/you/termask && cd termask
make install          # → ~/.local/bin/termask

termask init          # Anthropic API Key einrichten (Standard)
~/.local/bin/termask shell --shell zsh >> ~/.zshrc && source ~/.zshrc
```

## Provider hinzufügen

```bash
termask init --provider openai    # OpenAI
termask init --provider groq      # Groq (Free Tier!)
termask init --provider ollama    # Lokale Modelle (kein API Key)
termask init --provider together  # Together AI

termask switch groq               # Standard wechseln
```

## Alle Befehle

| Befehl | Beschreibung |
|--------|-------------|
| `termask ask "frage"` | Frage stellen (Standard-Provider) |
| `termask ask -p openai "frage"` | Anderen Provider für diese Anfrage |
| `termask ask --continue "folgefrage"` | Letzte Conversation fortsetzen |
| `termask ask --template shell "frage"` | Prompt-Vorlage verwenden |
| `termask ask --file main.go "review"` | Datei explizit als Kontext anhängen |
| `termask chat` | Multi-Turn Chat im Terminal |
| `termask tui` | Optionale einfache Terminal UI |
| `termask history list` | Gespeicherte Sessions anzeigen |
| `termask history show <id>` | Session als Markdown anzeigen |
| `termask history export <id> out.md` | Session als Markdown exportieren |
| `termask templates` | Built-in und eigene Prompt-Vorlagen anzeigen |
| `termask doctor` | Config und Setup prüfen |
| `termask config get <key>` | Config-Wert lesen |
| `termask config set <key> <value>` | Config-Wert setzen |
| `termask switch <provider>` | Standard-Provider dauerhaft wechseln |
| `termask switch --interactive` | Provider interaktiv auswählen |
| `termask providers` | Alle Provider + Status anzeigen |
| `termask models` | Verfügbare Modelle auflisten |
| `termask models --select` | Modell interaktiv auswählen und speichern |
| `termask models -p ollama` | Modelle eines bestimmten Providers |
| `termask init` | Provider interaktiv einrichten |
| `termask shell --shell zsh` | Shell-Plugin ausgeben |

## Shell-Shortcut: Provider wechseln

```bash
# Temporär anderen Provider nutzen:
TERMASK_PROVIDER=ollama  # dann Ctrl+K

# Oder dauerhaft:
termask switch openai
```

## Neuen Provider hinzufügen (OpenAI-kompatibel)

Jeder OpenAI-kompatible Endpoint funktioniert — einfach in `main.go` registrieren:

```go
provider.Register(openaiprovider.NewCompatible(
    "mistral",
    "https://api.mistral.ai/v1/chat/completions",
    []provider.Model{
        {ID: "mistral-large-latest", Description: "Mistral Large"},
        {ID: "mistral-small-latest", Description: "Mistral Small"},
    },
))
```

Dann in `config.toml`:
```toml
[providers.mistral]
api_key = "..."
model   = "mistral-large-latest"
```

## Konfigurationsdatei

`~/.config/termask/config.toml` — Berechtigungen: `0600` (nur für den User lesbar).
Alternativ: `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GROQ_API_KEY`, `TOGETHER_API_KEY` als Umgebungsvariablen.

## Conversation History

Wenn `history_enabled = true` gesetzt ist, speichert termask Sessions lokal in
`~/.local/share/termask/history.jsonl`. Das ermöglicht:

```bash
termask ask --continue "und als portable Variante?"
termask chat
termask history list
termask history export <id> session.md
```

## Prompt-Vorlagen

Built-ins:

```bash
termask templates
termask templates show shell
termask ask --template debug < error.log
```

Eigene Vorlagen können in der Config ergänzt werden:

```toml
[templates.shell-safe]
description = "Shell command with safety notes"
prompt = "Give a safe shell command first, then short safety notes.\n\nUser request: {{input}}"
```

## Project Context

Dateien werden nur explizit angehängt, damit keine ungewollten Projektinhalte an Provider gesendet werden:

```bash
termask ask --file README.md --file cmd/termask/main.go "was würdest du verbessern?"
```

## Eigene OpenAI-kompatible Provider

```toml
[providers.mistral]
type     = "openai-compatible"
api_key  = "..."
base_url = "https://api.mistral.ai/v1/chat/completions"
model    = "mistral-large-latest"

[[providers.mistral.models]]
id = "mistral-large-latest"
description = "Mistral Large"
```
