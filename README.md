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
termask shell --shell zsh >> ~/.zshrc && source ~/.zshrc
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
| `termask switch <provider>` | Standard-Provider dauerhaft wechseln |
| `termask providers` | Alle Provider + Status anzeigen |
| `termask models` | Verfügbare Modelle auflisten |
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
