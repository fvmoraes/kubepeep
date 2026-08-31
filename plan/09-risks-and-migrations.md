# Riscos e Estratégias de Migração do KubePeep

## 1. Riscos técnicos

### R1 — Regressões visuais no frontend

**Descrição:** a adoção de design system e componentes atômicos pode alterar a aparência de telas.

**Impacto:** experiência do usuário degradada; quebra de E2E baseados em seletores visuais.

**Mitigação:**
- Estabelecer baseline de screenshots com Playwright antes das mudanças.
- Migrar incrementalmente, uma tela por vez.
- Manter classes CSS antigas como fallback durante a transição.

**Rollback:** reverter para `styles.css` anterior e componentes originais.

---

### R2 — Mudanças arquiteturais quebram contratos

**Descrição:** centralizar ports e consolidar dashboard/resources pode alterar DTOs ou comportamento de rotas.

**Impacto:** frontend quebra; testes E2E falham.

**Mitigação:**
- Manter rotas e DTOs estáveis.
- Refatorar apenas internamente nos primeiros passos.
- Adicionar testes de contrato antes das refatorações.

**Rollback:** manter interfaces antigas como aliases temporários.

---

### R3 — Testes flaky permanecem

**Descrição:** correção de races pode introduzir novos deadlocks ou condições de corrida.

**Impacto:** CI instável; confiança reduzida.

**Mitigação:**
- Usar primitivas de sincronização robustas.
- Executar testes afetados centenas de vezes localmente.
- Adicionar `goleak` em testes de longa duração.

**Rollback:** reverter alteração no cache de autorização ou sincronização.

---

### R4 — Performance em grandes clusters

**Descrição:** parser composto de busca, painel lateral e novos blocos do dashboard podem aumentar carga no API server.

**Impacto:** lentidão; timeouts; negação de serviço indireta.

**Mitigação:**
- Manter budgets e limites documentados.
- Testar com Kind configurado com muitos recursos.
- Implementar cancelamento e timeouts em todas as novas operações.

**Rollback:** desabilitar parser composto ou novos blocos via feature flag.

---

### R5 — Segurança comprometida por melhorias de UX

**Descrição:** YAML highlight, terminal avançado, diff ou busca global podem inadvertidamente expor dados sensíveis.

**Impacto:** vazamento de Secret, logs ou credenciais.

**Mitigação:**
- Manter Secret metadata-only sem YAML.
- Sanitizar todo conteúdo antes de highlight/display.
- Não indexar logs/YAML/Secret em busca global.
- Executar inspeção negativa em cada mudança.

**Rollback:** desabilitar funcionalidade; revogar/rotacionar credenciais se necessário.

---

### R6 — Build desktop quebra

**Descrição:** mudanças no frontend ou na composição do core podem afetar o build Wails.

**Impacto:** binário desktop não compila.

**Mitigação:**
- Testar `go build ./...` com e sem tag `desktop`.
- Validar desktop em runner nativo quando possível.
- Documentar dependências nativas.

**Rollback:** reverter mudanças que quebrarem a tag `desktop`.

---

## 2. Riscos de produto

### R7 — Experiência do Aptakube sem copiar identidade

**Descrição:** equipe pode inclinar-se a copiar elementos visuais ou fluxos do Aptakube.

**Impacto:** risco legal/ético; perda de identidade própria.

**Mitigação:**
- Usar Aptakube apenas como benchmark funcional.
- Documentar explicitamente o que é inspirado e o que é original.
- Revisar designs antes da implementação.

---

### R8 — Escopo da Fase 9 cresce

**Descrição:** muitos facilitadores de UX podem alongar o MVP.

**Impacto:** atraso na release; acúmulo de débito.

**Mitigação:**
- Manter gates da Fase 9 separados do MVP.
- Priorizar P0/P1; deixar P3 para evoluções futuras.
- Não introduzir edição/aplicação genérica de YAML.

---

## 3. Estratégias de migração

### 3.1 Migração para design system

1. Criar tokens e componentes em paralelo ao código existente.
2. Substituir gradualmente, começando pelos componentes mais simples.
3. Manter screenshots baselines.
4. Remover CSS antigo apenas após 100% da migração.

### 3.2 Migração para ports centralizados

1. Criar interfaces em `internal/ports/`.
2. Fazer serviços existentes implementarem as interfaces (sem mudança de comportamento).
3. Alterar handlers para depender das interfaces.
4. Remover interfaces antigas dispersas.

### 3.3 Migração dashboard/resources

1. Extrair classificadores para pacotes compartilhados.
2. Fazer dashboard usar os mesmos ports/classificadores.
3. Comparar saídas antes e depois (golden tests).
4. Remover implementações duplicadas.

### 3.4 Migração de testes flaky

1. Identificar causa raiz com `-race`.
2. Reproduzir isoladamente.
3. Aplicar correção mínima.
4. Executar 100x para validar.

## 4. Planos de rollback

| Mudança | Gatilho de rollback | Procedimento |
| --- | --- | --- |
| Design system | Regressões visuais significativas | Reverter `styles.css` e componentes afetados. |
| Ports centralizados | Quebra de contrato | Manter aliases para interfaces antigas. |
| Dashboard/resources | Mudança de comportamento | Restaurar implementações separadas. |
| Parser composto | Performance inaceitável | Feature flag para substring simples. |
| YAML highlight | Aumento excessivo de bundle | Remover biblioteca. |
| Terminal xterm.js | Problemas de segurança/performance | Voltar a `<pre>`. |
| OTel/métricas | Tráfego não esperado | Desabilitar via configuração. |

## 5. Checklist de mitigação

- [ ] Baseline de screenshots estabelecida.
- [ ] Testes de contrato criados antes das refatorações arquiteturais.
- [ ] `-race` executado e estável.
- [ ] Kind usado para validar caminhos reais.
- [ ] Inspeção negativa de segurança executada.
- [ ] Build desktop validado.
- [ ] Documentação atualizada junto com cada mudança.
