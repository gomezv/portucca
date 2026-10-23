# AGENTS.md — portucca-internal

Repo privado de documentación interna de `portucca`. El código vive en el repo público
`/home/shuy/Proyectos/Skynet/portucca` (`gomezv/portucca`).

## Reglas

1. Aquí solo viven documentos: SPEC, DESIGN, DECISIONS, ROADMAP, COMPETITORS, PUBLISHING y
   ADOPTION. No se escribe código en este repo.
2. El código, los tests y los PRs van en el repo público. Sus reglas están en
   `../portucca/AGENTS.md`.
3. Toda decisión con alternativas se anota en `docs/DECISIONS.md` antes de codificarse.
4. Nunca se copian documentos internos al repo público ni se enlaza desde él a este repo.
5. Documentación en español; código en inglés.
6. Linux primero, fail-fast, cero dependencias de runtime en v1: reglas del código que se
   aplican en el repo público.

## Al tocar la política de puertos

Actualiza en el mismo cambio: `docs/SPEC.md` (comportamiento observable), `docs/DESIGN.md`
(motivo) y `docs/DECISIONS.md` (bitácora). La especificación es el contrato; el código del
repo público es una implementación de ella.
