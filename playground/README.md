Playground for actionlint
=========================

This is a development directory for [actionlint playground](https://rhysd.github.io/actionlint/).

The playground is built with HTML/CSS/TypeScript/Wasm. All dependencies are defined in `package.json` and managed by `npm`.
Tasks for development are defined in [`Makefile`](./Makefile).

Use npm 11.10.0 or newer and install the committed dependency versions with `npm ci`. The repository enforces
package engine requirements and a seven-day minimum release age. Dependency lifecycle scripts are disabled in `.npmrc`;
the build invokes the first-party `post-install.bash` asset-copying script explicitly. Dependency updates are reviewed
through the repository's shared Astral Renovate policy.

## Tasks

```sh
# Install dependencies, build main.wasm, start serving the app at localhost:1234 using Python
make

# Install dependencies, build main.wasm
make build

# Install dependencies
make dep

# Run tests
make test

# Clean all built files and dependencies
make clean
```

## Lint

Sources are linted with [eslint](https://eslint.org/) with [typescript-eslint](https://github.com/typescript-eslint/typescript-eslint),
[prettier](https://prettier.io/) and [stylelint](https://stylelint.io/).

`lint` npm script applies all the liters:

```sh
npm run lint
```

## Deployment

Deployment is automated by [`deploy.bash`](./deploy.bash). See [CONTRIBUTING.md](../CONTRIBUTING.md) for more details.
To optimize `main.wasm`, `wasm-opt` command is required. Install [Binaryen](https://github.com/WebAssembly/binaryen) in
advance.
