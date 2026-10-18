# Tabular Recipe Notation Renderer

The core engine of [TRN](https://github.com/aadjm/trn): a pure Go (stdlib-only)
renderer that turns the frozen TRN markdown dialect into standalone, self-styled
HTML — Cooking-for-Engineers-style matrix and flow views. No network, no
filesystem access beyond the input, no state, no JavaScript in its output.

- **Contract**: [`SPEC-ENGINE.md`](https://github.com/aadjm/trn/blob/main/docs/SPEC-ENGINE.md)
  in the `aadjm/trn` repo is the source of truth for the dialect, the library API,
  and the acceptance criteria (E-A1 … E-A6).
- **Consumers**: the TRN site (`aadjm/trn`) imports this module as a library; the
  `trn-render` CLI renders files standalone.
- **Implementation**: tracked as Kanban cards `TRN-ENG-1` … `TRN-ENG-8`.
- **Constraints (non-negotiable)**: Go stdlib only; never panic on input; HTML
  generated via `html/template` escaping paths.

## Usage (once implemented)

```console
$ trn-render recipe.trn.md --standalone > recipe.html
```

## License

MIT
