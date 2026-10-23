# Adopción en SEUCCA

SEUCCA es el primer consumidor de `portucca` fuera de `shucca`. Esta nota deja la
adaptación resuelta antes de escribir el código del repo público.

## 1. Contexto

SEUCCA (`/home/shuy/Proyectos/Skynet/seucca`) está en fase de scaffold: solo existe
`docs/` (spec, diseño y plan del walking skeleton). El scaffold define su propio stack
(NestJS, Prisma 7, React/Vite, Refine) y su propio contrato de puertos, y **no** contempla
un flujo de worktrees.

Lo que sí comparte con `shucca` es el origen de la política: `shucca` PR #22
(`feat(scripts): puertos de dev por worktree`, mergeado el 2026-10-22) resolvió el
problema de los worktrees con puertos sticky, un registro con `flock` y guards que fallan
duro. SEUCCA arranca con esa política desde el día uno, en lugar de rediscoverla.

## 2. Bandas de SEUCCA

| Servicio | Main (reservado) | Worktrees |
|---|---|---|
| Web | 5313 | 5314+ |
| API | 3314 | 3315+ |

Motivo: las familias **no pueden coincidir** con las de `shucca` (`5173`/`3004` +
worktrees `5174+`/`3005+`, que hoy ya ocupan `5174–5182` y `3005–3013` y cuyo escaneo
sigue subiendo sin límite). El margen deja ≈130 slots en web y ≈300 en API entre ambas
bandas; si alguna vez se cruzan, el conflicto es ruidoso (`EADDRINUSE`), nunca silencioso.
La garantía formal llegará cuando `shucca` migre al registro global
(`docs/DECISIONS.md`, D5).

## 3. Cambios en el scaffold de SEUCCA

Nada de esto existe todavía: son los puntos donde el plan del scaffold debe incorporar la
política.

### Task 1 — base del monorepo

- Incluir `scripts/` y `.kilo/setup-script` en los archivos a crear.
- `.gitignore` debe cubrir `apps/**/.env.local`, `apps/api/.env`, `node_modules/` y
  `packages/prisma/generated/`.
- `.env.example`: `PORT=3314`, `CORS_ORIGIN=http://localhost:5313`.
- Script raíz de la capa 0: `test:scripts: "node --test scripts/*.test.mjs"`.
- Actualizar el criterio **AC-1** del plan: `pnpm dev` levanta la API en `:3314` y la web
  en `:5313` (el plan actual dice `:3004` y `:5173`).

### Task 4 — bootstrap de la API

- `ConfigModule.forRoot` con `envFilePath` **absolutas** y `.env.local` primero:
  `[path.resolve(__dirname, '..', '.env.local'), path.resolve(__dirname, '..', '.env')]`.
- Guard de arranque (solo dev): claves JWT faltantes, worktree intentando el puerto del
  main, y `PrismaClientInitializationError` traducido a mensaje accionable.
- CORS: regex de loopback en dev (el puerto del worktree es variable), allowlist explícita
  en prod; nunca `origin: true` con `credentials: true`.

### Task 10 — base web

- `server.port` desde `WEB_PORT` (default `5313`), `proxy` hacia `API_PROXY_TARGET`
  (default `http://localhost:3314`).
- `server.strictPort: true` y `preview.strictPort: false` explícito.
- Guard: si el repo es un worktree y el puerto es el del main, Vite falla en `serve`
  nombrando el script de setup. En `build` el guard no corre.

### Task 13 — CI y documentación

- `pnpm test:scripts` en el job `checks` del pipeline.
- `AGENTS.md` con la política de puertos, las bandas y la regla de los worktrees.
- `docs/DEV-PORTS.md` en SEUCCA con la política local.

## 4. Base de datos: compartida por ahora

Los worktrees de SEUCCA comparten la base de datos. No es por descuido: el aislamiento por
`tenantId` protege **filas** de una misma institución, no el estado del schema entre
ramas. El riesgo real es que `prisma migrate dev` desde ramas divergentes detecte *drift*
y proponga resetear la base (la documentación de Prisma lo dice: "intended to be used only
in development with a disposable database").

Mientras tanto:

- Solo una rama toca schema a la vez.
- Los worktrees usan `migrate deploy`, no `migrate dev`.
- Prisma bloquea `migrate reset --force` a los agentes sin consentimiento explícito
  (`PRISMA_USER_CONSENT_FOR_DANGEROUS_AI_ACTION`).

La opción de menor costo cuando se active la base por rama es un `CREATE DATABASE
<rama>` en el mismo Postgres: sin contenedores adicionales.

## 5. Checklist de aceptación (adaptación en SEUCCA)

- [ ] `git init` + commit inicial de la documentación (sin commit no hay worktrees)
- [ ] `scripts/` presente y `node --test scripts/*.test.mjs` en verde
- [ ] `scripts/setup-worktree.sh` idempotente: segunda corrida con `changed: false`
- [ ] Worktree temporal: reclama `5314`/`3315`, escribe `apps/web/.env.local` y
      `apps/api/.env.local`
- [ ] El symlink `apps/api/.env -> ../../.env` se crea y no pisa un `.env` existente
- [ ] Checkout principal: `ensure` reporta `isMain: true` y no escribe nada
- [ ] `prune` elimina claims de directorios inexistentes
- [ ] Los ports de `shucca` no se ven afectados por la operación de SEUCCA
- [ ] El plan del scaffold refleja los cambios de las Tasks 1, 4, 10 y 13
