# Diseño técnico de portucca

Derivado del diseño de `shucca` (PR #22) y de las correcciones que salieron en el proceso. Cada decisión dice **qué** y **por qué**; las alternativas
descartadas están en `docs/DECISIONS.md`.

## 1. Decisiones

| # | Decisión | Motivo |
|---|---|---|
| D1 | Los puertos se asignan **al crear el worktree** y no se reasignan en cada arranque | La URL de un worktree es estable: bookmarks, gates de revisión y prefill siguen funcionando |
| D2 | Series **desacopladas** para web y API | Si el puerto de la API está ocupado, eso no obliga a mover el de la web |
| D3 | El checkout principal queda **reservado** (no entra al registro) | Un worktree nunca puede robarle el puerto al main |
| D4 | El puerto efectivo vive en **`.env.local`** del proyecto | Vite y NestJS lo leen de ahí; sobrevive a reinicios y a otros shells |
| D5 | `strictPort: true` en el servidor web; sin fallback en la API | Ante conflicto, error explícito en lugar de mudanza silenciosa |
| D6 | **Sin reasignación automática** al arrancar | Si el puerto está ocupado, el error dice qué correr (`--reallocate`) |
| D7 | La política vive **en el repositorio**, no en un hook oculto | La política de shucca vivía en un SQLite de Orca, invisible para el repo: nadie la podía revisar |
| D8 | El claim sigue al **directorio** del worktree, no al proceso | Un worktree detenido conserva su puerto; se libera al borrar el directorio |
| D9 | CORS de dev por regex de loopback; allowlist explícita en prod | El puerto del worktree es variable; `origin: true` con credenciales queda prohibido |
| D10 | El registro es **global a la máquina** con claim etiquetado por proyecto | Dos proyectos que quieren `5173` no se pisan: el problema real detectado en SEUCCA |

## 2. Asignación de puertos

### 2.1 Detección de main vs worktree

`git rev-parse --git-dir` frente a `--git-common-dir`: si coinciden, es el checkout
principal; si no, es un worktree. No se usan heurísticas sobre el nombre de la carpeta ni sobre
la presencia de `.git` (un archivo vs un directorio) para decidir el *rol*; ese segundo
método solo se usa en los guards, donde es suficiente y barato.

### 2.2 Orden de decisión de `ensure`

1. Cargar el registro bajo `flock` exclusivo.
2. Podar claims cuyo directorio ya no existe.
3. Si hay claim propio: no reasignar. Reparar `.env.local` si falta o no coincide.
4. Si no hay claim: intentar **adopción** de los puertos que ya están en `.env.local`
   (siempre que estén dentro de la banda del proyecto y no los reclame otro worktree vivo).
   La adopción **no** hace test de bind: el stack puede estar arriba y ese es el caso
   legítimo.
5. Si no hay nada adoptable: recorrer la banda del proyecto desde el inicio y tomar el
   primer puerto que (a) no esté reclamado por otro worktree vivo y (b) esté libre de
   verdad.
6. Registrar el claim y escribir los `.env.local` de forma atómica.

### 2.3 "Libre de verdad"

Se hace bind en `0.0.0.0` **y** en `::` con `node:net`. Vite escucha en `0.0.0.0` y Node en
`::` (dual-stack): probar una sola familia puede dar un falso "libre" que se rompe al
arrancar. Nunca se usa `ss`/`netstat`: fallar cerrado es mejor que interpretar mal la
salida de otra herramienta.

### 2.4 TOCTOU entre el test de bind y el arranque

No se puede cerrar por completo: entre el bind test y el arranque, otro proceso puede
tomar el puerto. La respuesta no es reintentar (eso reintroduce la mudanza silenciosa),
sino **fallar fuerte**: `strictPort` en Vite produce `EADDRINUSE` y la API muere con el
mismo código. El error es la red de seguridad; el registro es la mejor-effort.

## 3. Escrituras

| Aspecto | Decisión | Motivo |
|---|---|---|
| Atomicidad | temporal + `rename` | Nunca un `.env.local` a medio escribir |
| Merge | se preservan claves ajenas | `VITE_DEMO_*` y las de la app sobreviven a una reasignación |
| Orden de precedencia | el allocator escribe `WEB_PORT`/`API_PROXY_TARGET`/`PORT`; el prefill escribe `VITE_DEMO_*` | Sin colisión de escritores |
| Permisos | registro `0600`, directorio `0700` | El registro describe la topología de la máquina |
| Precedencia de carga | `.env.local` **antes** que `.env` | En `@nestjs/config` gana el primero del array; en Vite es al revés |

## 4. Guards

| Guard | Momento | Condición | Mensaje |
|---|---|---|---|
| API: entorno incompleto | después de `NestFactory.create()` (el entorno se carga durante el create) | faltan claves JWT | nombra `portucca setup` |
| API: worktree en el puerto del main | idem | es worktree y el puerto resuelto es el del main | nombra `portucca setup` |
| API: base de datos inaccesible | idem | `PrismaClientInitializationError` | explica que falta `DATABASE_URL` o el symlink de `.env` |
| Web: worktree en el puerto del main | al configurar Vite (`command === 'serve'`) | es worktree y el puerto es el del main | nombra `portucca setup` |

Los guards son **solo de desarrollo**: en producción no hay `.git` y no hay worktree, y el
proceso no debe romperse por una comprobación de desarrollo.

## 5. Carga del entorno en la API

`ConfigModule.forRoot` con rutas **absolutas**:

```ts
envFilePath: [
  path.resolve(__dirname, '..', '.env.local'),
  path.resolve(__dirname, '..', '.env'),
]
```

Con un array, `@nestjs/config` no resuelve rutas relativas (solo resuelve el default), y
`__dirname` funciona tanto desde `src/` como desde `dist/` porque `outDir` es `./dist` y el
módulo es CommonJS.

## 6. Límites conocidos

| Límite | Impacto | Mitigación |
|---|---|---|
| Linux only (`flock`) | macOS/Windows sin soporte en v1 | Fallar con mensaje claro; alternativa futura: `proper-lockfile` |
| Sin `--reallocate` el claim nunca se libera solo | Un worktree movido pierde su puerto | `--reallocate` documentado; el claim se indexa por `realpath` |
| La adopción ignora el estado real del puerto | Un puerto ocupado por un proceso ajeno se adopta | Se detecta al arrancar (`EADDRINUSE`); `--reallocate` corrige |
| Sin base de datos por worktree | `prisma migrate dev` desde ramas divergentes puede pedir resetear la base | Decisión diferida (`docs/DECISIONS.md`, D7) |
| El registro no detecta procesos zombis | Un claim de un worktree borrado se libera al prune | `prune` corre en el siguiente `ensure` |
