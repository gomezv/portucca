# portucca-internal

Repositorio privado de trabajo de **portucca**.

- `public/`: lo único que se publica, como espejo, en <https://github.com/gomezv/portucca>.
- `docs/`: especificación, diseño, decisiones, roadmap, competencia y notas de adopción. No se publica.
- `.github/workflows/sync-public.yml`: sincroniza `public/` con el repo público al mergear en `main`.

## Cómo se trabaja

1. Rama + PR contra `main` de este repo.
2. El código y la documentación pública se editan en `public/`.
3. Las decisiones y el diseño se editan en `docs/`.
4. Al mergear, el workflow hace `git subtree split --prefix=public` y empuja a `gomezv/portucca`.

## Verificación

```bash
cd public
npm test
npm pack --dry-run
```

## Documentos internos

- [`docs/SPEC.md`](docs/SPEC.md)
- [`docs/DESIGN.md`](docs/DESIGN.md)
- [`docs/DECISIONS.md`](docs/DECISIONS.md)
- [`docs/ROADMAP.md`](docs/ROADMAP.md)
- [`docs/COMPETITORS.md`](docs/COMPETITORS.md)
- [`docs/PUBLISHING.md`](docs/PUBLISHING.md)
- [`docs/ADOPTION-SEUCCA.md`](docs/ADOPTION-SEUCCA.md)
