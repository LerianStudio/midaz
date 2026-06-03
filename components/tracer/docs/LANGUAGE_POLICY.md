# Language Policy - English Only

**All project artifacts MUST be written in English.**

This policy ensures:
- Code readability for international collaborators
- Consistency across documentation
- Professional standards
- Better tooling support (linters, AI assistants)

---

## What Must Be in English

### Code
- ✅ Variable names
- ✅ Function names
- ✅ Type names
- ✅ Constants
- ✅ Package names
- ✅ Interface names

### Comments
- ✅ Inline comments (`// comment`)
- ✅ Block comments (`/* comment */`)
- ✅ Doc comments (godoc)
- ✅ TODOs and FIXMEs
- ✅ Code annotations

### Documentation
- ✅ README files
- ✅ Technical documentation
- ✅ Architecture documents
- ✅ API documentation
- ✅ User guides

### Git
- ✅ Commit messages
- ✅ PR titles
- ✅ PR descriptions
- ✅ Branch names
- ✅ Tag descriptions

### Runtime
- ✅ Error messages
- ✅ Log messages
- ✅ User-facing messages
- ✅ Debug output

### Testing
- ✅ Test names
- ✅ Test descriptions
- ✅ Assertion messages
- ✅ Test documentation

---

## Examples

### ✅ CORRECT - English

```go
// CreateRule creates a new validation rule with the given parameters.
// Returns error if name is empty or expression is invalid.
func CreateRule(name, expression string, action Decision) (*Rule, error) {
    if name == "" {
        return nil, errors.New("rule name is required")
    }
    
    // Normalize name by trimming whitespace
    normalizedName := strings.TrimSpace(name)
    
    return &Rule{
        Name:       normalizedName,
        Expression: expression,
        Action:     action,
    }, nil
}
```

```bash
# Commit message
git commit -m "feat: add validation for empty scopes

Validate that scopes are not empty before mutating rule state.
Returns ErrRuleInvalidScope if any scope has all nil fields.

Tests:
- Add 7 test cases covering edge cases
- Verify atomicity (no partial mutation on failure)"
```

```go
// Test
func TestCreateRule_EmptyName_ReturnsError(t *testing.T) {
    t.Parallel()
    
    _, err := CreateRule("", "amount > 100", DecisionAllow)
    
    require.Error(t, err)
    assert.ErrorIs(t, err, ErrRuleNameRequired)
}
```

### ❌ INCORRECT - Portuguese

```go
// CriaRegra cria uma nova regra de validação com os parâmetros fornecidos.
// Retorna erro se nome está vazio ou expressão é inválida.
func CriaRegra(nome, expressao string, acao Decision) (*Regra, error) {
    if nome == "" {
        return nil, errors.New("nome da regra é obrigatório")
    }
    
    // Normaliza nome removendo espaços
    nomeNormalizado := strings.TrimSpace(nome)
    
    return &Regra{
        Nome:      nomeNormalizado,
        Expressao: expressao,
        Acao:      acao,
    }, nil
}
```

```bash
# Mensagem de commit
git commit -m "feat: adiciona validação de scopes vazios

Valida que scopes não estão vazios antes de mutar estado.
Retorna ErrRuleInvalidScope se algum scope tem campos nil."
```

---

## Enforcement

### 1. Code Review

**Reviewers MUST:**
- ❌ Reject PRs with non-English code/comments/messages
- ✅ Request translation before approval
- ✅ Provide constructive feedback

**Example review comment:**
```
Please translate all Portuguese comments and variables to English:
- "nome" → "name"
- "acao" → "action"
- "// Valida se..." → "// Validates if..."
```

### 2. Git Commit Hook (Optional)

Create `.git/hooks/commit-msg`:

```bash
#!/bin/bash
# Check commit message for non-ASCII characters

COMMIT_MSG_FILE=$1
COMMIT_MSG=$(cat "$COMMIT_MSG_FILE")

# Check for non-ASCII characters (covers most non-English text)
if echo "$COMMIT_MSG" | grep -qP '[^\x00-\x7F]'; then
    echo "❌ ERROR: Commit message contains non-English characters"
    echo ""
    echo "Policy: All commit messages must be in English"
    echo "See: docs/LANGUAGE_POLICY.md"
    echo ""
    echo "Your message:"
    echo "$COMMIT_MSG"
    exit 1
fi

# Check for common Portuguese words
PORTUGUESE_WORDS="adiciona|remove|corrige|atualiza|implementa|cria|modifica"
if echo "$COMMIT_MSG" | grep -qiE "$PORTUGUESE_WORDS"; then
    echo "⚠️  WARNING: Commit message may contain Portuguese words"
    echo ""
    echo "Common translations:"
    echo "  adiciona → add"
    echo "  remove → remove"
    echo "  corrige → fix"
    echo "  atualiza → update"
    echo "  implementa → implement"
    echo "  cria → create"
    echo "  modifica → modify"
    echo ""
    echo "Continue? (y/N)"
    read -r response
    if [[ ! "$response" =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

exit 0
```

Make executable:
```bash
chmod +x .git/hooks/commit-msg
```

### 3. golangci-lint (Partial Support)

Add to `.golangci.yml`:

```yaml
linters-settings:
  varnamelen:
    # Encourage English naming
    min-name-length: 3
  
  revive:
    rules:
      - name: var-naming
        severity: warning
        arguments:
          - ["ID", "HTTP", "URL", "API"]  # English acronyms
```

### 4. CI/CD Pipeline

Add check in `.github/workflows/code-quality.yml`:

```yaml
- name: Check for non-English content
  run: |
    # Check for non-ASCII in code files
    if find . -name "*.go" -exec grep -P '[^\x00-\x7F]' {} + | grep -v "Copyright"; then
      echo "❌ Non-ASCII characters found in code"
      exit 1
    fi
    
    # Check commit messages in PR
    if gh pr view --json commits | jq -r '.commits[].commit.message' | grep -P '[^\x00-\x7F]'; then
      echo "❌ Non-English commit messages found"
      exit 1
    fi
```

---

## Common Translations

### Portuguese → English

| Portuguese | English |
|-----------|---------|
| adicionar | add |
| remover | remove |
| corrigir | fix |
| atualizar | update |
| criar | create |
| modificar | modify |
| implementar | implement |
| refatorar | refactor |
| testar | test |
| validar | validate |
| erro | error |
| sucesso | success |
| falha | failure |
| obrigatório | required |
| opcional | optional |
| vazio | empty |
| inválido | invalid |
| nome | name |
| ação | action |
| regra | rule |
| escopo | scope |

### Commit Message Patterns

| Portuguese | English |
|-----------|---------|
| feat: adiciona X | feat: add X |
| fix: corrige Y | fix: fix Y |
| refactor: refatora Z | refactor: refactor Z |
| test: adiciona testes | test: add tests |
| docs: atualiza documentação | docs: update documentation |
| chore: remove código morto | chore: remove dead code |

---

## Exceptions

**ONLY exception:** User-facing strings in a localized application.

```go
// ✅ ACCEPTABLE - User-facing localization
var messages = map[string]string{
    "pt-BR": "Nome é obrigatório",
    "en-US": "Name is required",
}

// But code and comments MUST still be English:
// ValidateName checks if the name field is not empty.
func ValidateName(name string, locale string) error {
    if strings.TrimSpace(name) == "" {
        return errors.New(messages[locale])
    }
    return nil
}
```

**Note:** The Tracer project currently does NOT support localization, so this exception does not apply.

---

## Benefits

1. **International Collaboration** - Team members from any country can contribute
2. **Tool Support** - Linters, AI assistants work better with English
3. **Industry Standard** - English is the lingua franca of programming
4. **Code Reusability** - Easier to share and reuse code
5. **Documentation** - One language for all docs reduces maintenance
6. **Hiring** - Attracts talent globally

---

## Migration Guide

### For Existing Code

1. **Identify non-English code:**
   ```bash
   # Find non-ASCII in Go files
   find . -name "*.go" -exec grep -P '[^\x00-\x7F]' {} +
   ```

2. **Create translation task list:**
   - List all files with non-English content
   - Prioritize public APIs and documentation
   - Schedule gradual translation

3. **Translate in phases:**
   - Phase 1: Public APIs and exported functions
   - Phase 2: Internal functions and comments
   - Phase 3: Test names and descriptions
   - Phase 4: Historical commit messages (optional)

### For New Code

**ALWAYS write in English from the start.**

Use this checklist before committing:
- [ ] All variables/functions in English?
- [ ] All comments in English?
- [ ] Commit message in English?
- [ ] Test names in English?
- [ ] No accented characters (á, é, í, ó, ú, ã, õ, ç)?

---

## Getting Help

### Translation Resources

- **Google Translate:** https://translate.google.com/
- **DeepL (better quality):** https://www.deepl.com/
- **Programming glossary:** https://github.com/github/glossary
- **Ask team:** Post in #engineering-help channel

### Common Mistakes

1. **Direct translation:** Some phrases don't translate literally
   - ❌ "is obligatory" (literal)
   - ✅ "is required" (idiomatic)

2. **Technical terms:** Use established English terms
   - ❌ "factory of objects"
   - ✅ "object factory" or "factory pattern"

3. **Comments:** Keep them concise and clear
   - ❌ "This function is doing the validation of the rule"
   - ✅ "Validates the rule"

---

## Questions?

Contact the Engineering Team or post in #engineering-help.

**Remember:** When in doubt, ask! It's better to ask than to commit non-English code.

---

**Last Updated:** 2026-02-04  
**Version:** 1.0  
**Maintained by:** Engineering Team
