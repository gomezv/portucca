# Bitácora de decisiones

Formato: fecha · decisión · contexto · alternativas descartadas. Las decisiones con
alternativas reales se anotan **antes** de codificarse.

---

## D1 · Nombre del repositorio: `portucca`

**Fecha:** 2026-10-22

El proyecto es parte de la familia `sh-ucca` / `se-ucca`. La regla de la casa es
`<dominio> + ucca`, así que la herramienta de puertos es `port-ucca`.

- **Elegido:** `portucca` — dice qué hace y mantiene el patrón de familia.
- **Alternativa descartada:** `ucca` — la raíz de la familia; no dice qué hace y en GitHub
  se ve como "un repo más". Sigue siendo el nombre de reserva si el scope crece a
  múltiples herramientas.
- **Alternativa descartada:** `devucca` / `ucca-dev` / `worktree-ucca` — "dev" es ruido y
  `worktree` limita el alcance antes de tiempo.

**Verificado:** `portucca` libre en npm y `github.com/gomezv/portucca` libre (404) el
2026-10-22. El nombre de un paquete npm es irreversible en la práctica (tras 72h de
despublicación queda bloqueado para siempre), de ahí el nombre en el paquete y no solo en
el repo.

---

## D2 · Slogan

**Fecha:** 2026-10-22

- **Capa de familia (README, GitHub About):** `Shucca, Seucca, Portucca: cada uno en su puerto.`
- **Capa técnica (descripción de npm):** `Un puerto estable por worktree. Sticky, auditable, sin EADDRINUSE.`
- **Candidatos guardados:** "Todos los puertos los tiene… su worktree" (inacabado a
  propósito, funciona como gancho en la bio de la org) y "Cada worktree, su puerta".

---

## D3 · Publicar en npm

**Fecha:** 2026-10-22

Un repo público que se quiere compartir se consume con `npx` o como dependencia. Sin npm
quedaría limitado a `git clone` o `npx github:...`, sin semver.

- **Elegido:** publicar en npm como `portucca`, y consumirlo en cada proyecto como
  **dependencia de desarrollo versionada** (`portucca@0.1.0`), no con `npx` en cada
  arranque: `npx` exige red en cada setup de worktree (y los agentes pueden correr sin
  red), mientras que la dependencia queda en `node_modules` del worktree y se instala con
  el `pnpm install` que ya ocurre.
- **Alternativa descartada:** `npx portucca@latest` en cada setup.
- **Alternativa descartada:** `install.sh` con GitHub Releases — funciona, pero pierde
  `npx` y el versionado.

---

## D4 · Orden: adaptación antes que publicación

**Fecha:** 2026-10-22

Primero se adapta la política de `shucca` a `seucca` y se prueba en los dos proyectos;
después se extrae al repo público; después se publica `0.1.0`.

- **Motivo:** los bordes (bandas, `.env` inexistente, symlink, guards) aparecen cuando dos
  proyectos reales lo usan, no cuando se publica.
- **Consecuencia:** el primer commit del repo público declara explícitamente que es un
  *port* de `shucca` PR #22, con atribución al repo y al PR de origen.

---

## D5 · Un solo registro global, con claim etiquetado por proyecto

**Fecha:** 2026-10-22

Al adaptarlo a SEUCCA se detectó que las familias de puertos **no pueden coincidir**:
`shucca` ocupa hoy `5174–5182` (web) y `3005–3013` (API), y su escaneo de puertos sigue
subiendo sin límite, así que una copia por proyecto terminaría chocando.

- **Elegido:** un registro global a nivel máquina (`$XDG_STATE_HOME/portucca/ports.json`)
  donde cada claim lleva su `project`. Dos proyectos que quieren `5173` no se pisan.
- **Alternativa descartada:** un registro por proyecto (el de `shucca` es por proyecto) —
  reintroduce el problema que se acaba de detectar.
- **Pendiente:** `shucca` debe migrar su registro al archivo global (PR pequeño, de bajo
  riesgo). Hasta entonces, `shucca` y `portucca` conviven con registros distintos y el
  conflicto entre proyectos se resuelve por bandas separadas.

---

## D6 · Bandas de puertos por proyecto

**Fecha:** 2026-10-22

| Proyecto | Web (main) | API (main) | Web (worktrees) | API (worktrees) |
|---|---|---|---|---|
| shucca | 5173 | 3004 | 5174+ | 3005+ |
| seucca | 5313 | 3314 | 5314+ | 3315+ |

- **Motivo:** margen amplio entre familias (≈130 slots en web, ≈300 en API) para que el
  escaneo de `shucca` no alcance la banda de SEUCCA en la práctica; si lo alcanzara, el
  conflicto es ruidoso (`EADDRINUSE`), nunca silencioso.
- **Pendiente:** cuando `shucca` migre al registro global, el margen deja de ser la
  garantía y pasa a serlo el registro.

---

## D7 · Base de datos por worktree: **diferido**, no descartado

**Fecha:** 2026-10-22

Decisión: por ahora **todos los worktrees comparten la misma base de datos**. Se revisa más
adelante, con la opción de activarse por proyecto.

- **Motivo de la duda (resuelta con evidencia):** no es el multitenant lo que protege aquí.
  El aislamiento por `tenantId` es de **filas** dentro de un mismo schema; un worktree es
  un **estado de schema distinto** (su `schema.prisma` y su historial de migraciones).
  Con dos ramas divergentes sobre la misma base, la documentación de Prisma es explícita:
  `prisma migrate dev` "is intended to be used only in development with a disposable
  database" y, si detecta *drift* o un conflicto de historial, "you will be prompted to
  reset (drop and recreate) your database" — lo que además dispara el seed y borra el
  trabajo del otro worktree.
- **Mitigaciones mientras tanto:** que solo una rama toque schema a la vez; que los
  worktrees usen `migrate deploy` y no `migrate dev`; y el guard de Prisma
  (`PRISMA_USER_CONSENT_FOR_DANGEROUS_AI_ACTION`) que bloquea `migrate reset --force` a los
  agentes sin consentimiento explícito.
- **Alternativa descartada ahora:** base de datos por rama en el mismo Postgres
  (`CREATE DATABASE <rama>`): es barato (una línea en el setup) y resuelve el crossfire de
  resets, pero se difiere para no romper con el flujo actual.
- **Nota de alcance:** cuando se active, la opción de menor costo es **una base por rama en
  el mismo Postgres**, no un contenedor por worktree.

---

## D8 · Licencia MIT

**Fecha:** 2026-10-22

El objetivo es que otros proyectos lo usen. MIT es la licencia con la que ya publican las
herramientas del ecosistema comparable (portweave, portlock, worktree-env, branch2ports) y
la que menos fricción pone a un consumidor corporativo.

---

## D9 · Linux first

**Fecha:** 2026-10-22

El bloqueo usa `flock` (util-linux). macOS y Windows quedan fuera de la v1 y deben fallar
con un mensaje claro. Alternativa futura si hay demanda: `proper-lockfile` (dependencia
npm) o un lock con `mkdir` atómico.

---

## D10 · El repositorio público no es todavía el repo de implementación

**Fecha:** 2026-10-22

Este repo arranca con la especificación y las decisiones. El código se escribe cuando la
adaptación a SEUCCA esté funcionando, para que el repo nazca con la política ya probada en
dos proyectos (ver D4).

**Actualizado por D11:** el repo de trabajo pasa a ser `portucca-internal`; el repo público
es un espejo de solo lectura.

---

## D11 · Repo privado de trabajo + espejo público

**Fecha:** 2026-10-22

El repo público no debe contener documentación de desarrollo, pero el trabajo diario
(ramas, commits y PRs) ocurre en un repo privado.

- **Elegido:** `portucca-internal` (privado) es el repo de trabajo. Contiene `public/` (lo
  único que se publica) y `docs/` (SPEC, DESIGN, DECISIONS, ROADMAP, COMPETITORS,
  PUBLISHING, ADOPTION). Al mergear en `main`, un workflow hace
  `git subtree split --prefix=public` y empuja a `gomezv/portucca`.
- **Motivo:** el público queda limpio y de solo lectura, sin docs internos ni historia de
  desarrollo; el trabajo diario y los PRs ocurren en un solo repo.
- **Alternativa descartada:** código en el repo público y docs en un repo privado separado —
  obliga a partir cada cambio en dos PRs y no era el flujo pedido.
- **Alternativa descartada:** copiar `public/` a mano en cada release — se desincroniza y no
  deja historial.
- **Consecuencia:** el público es un espejo; los PRs externos se importan al privado. Los
  commits que tocan `public/` deben tener mensajes públicos. El workflow necesita un PAT
  (`PUBLIC_REPO_TOKEN`) con escritura solo sobre el repo público.
