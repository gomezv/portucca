# AGENTS.md — portucca-internal

## Qué es este repo

Repo privado de trabajo de `portucca`. El repo público (`gomezv/portucca`) es un **espejo
de solo lectura** generado desde `public/`.

## Estructura

```
portucca-internal/
├── public/              # se publica tal cual en gomezv/portucca
│   ├── bin/
│   ├── lib/
│   ├── test/
│   ├── package.json
│   ├── README.md
│   ├── LICENSE
│   └── CONTRIBUTING.md
├── docs/                # interno: SPEC, DESIGN, DECISIONS, ROADMAP...
└── .github/workflows/sync-public.yml
```

## Reglas

1. **El código va en inglés**; la documentación en español.
2. **TypeScript/Node moderno**: ESM nativo (`"type": "module"`), Node >= 20. Sin
   transpilador: el paquete se distribuye tal cual.
3. **Cero dependencias de runtime** en v1. `node:net`, `node:fs`, `node:child_process` y
   `flock` son suficientes. Una dependencia se justifica solo con evidencia.
4. **Linux primero**: el bloqueo usa `flock` (util-linux). macOS/Windows quedan fuera de
   alcance en v1 y deben fallar con un mensaje claro, no con un error críptico.
5. **Fail-fast**: si el entorno no es válido (falta `flock`, puerto ocupado, claim de otro
   worktree), el proceso falla con un mensaje accionable. Nunca se degrada a un default
   silencioso ni se reasigna un puerto detrás de la espalda del usuario.
6. **Un solo origen de verdad**: el registro de claims es global a la máquina y etiquetado
   por proyecto. No se mantienen registros por proyecto.
7. **Sin secretos** en el repositorio. El registro de ports vive en
   `$XDG_STATE_HOME/portucca/ports.json` con permisos `0600` y nunca se versiona.
8. **Commits convencionales**, sin atribución de IA. Sin push ni PR sin pedido explícito.
9. **Toda decisión con alternativas** se anota en `docs/DECISIONS.md` antes de codificarse.
10. **`public/` es publicable**: nada interno, ningún enlace a `docs/`, mensajes de commit
    públicos en los cambios que tocan `public/`.
11. **El repo público no se edita a mano**: se regenera con `git subtree split`.

## Verificación

```bash
cd public
npm test
npm pack --dry-run
```

## Al tocar la política de puertos

Actualiza en el mismo cambio: `docs/SPEC.md` (comportamiento observable), `docs/DESIGN.md`
(motivo) y `docs/DECISIONS.md` (bitácora). La especificación es el contrato; el código es
una implementación de ella.
