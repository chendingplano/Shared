# Svelte library

Everything you need to build a Svelte library, powered by [`sv`](https://npmjs.com/package/sv).

Read more about creating a library [in the docs](https://svelte.dev/docs/kit/packaging).

## Creating a project

If you're seeing this, you've probably already done this step. Congrats!

```sh
# create a new project in the current directory
npx sv create

# create a new project in my-app
npx sv create my-app
```

## Developing

Once you've created a project and installed dependencies with `npm install` (or `pnpm install` or `yarn`), start a development server:

```sh
npm run dev

# or start the server and open the app in a new browser tab
npm run dev -- --open
```

Everything inside `src/lib` is part of your library, everything inside `src/routes` can be used as a showcase or preview app.

## Building

To build your library:

```sh
npm pack
```

To create a production version of your showcase app:

```sh
npm run build
```

You can preview the production build with `npm run preview`.

> To deploy your app, you may need to install an [adapter](https://svelte.dev/docs/kit/adapters) for your target environment.

## Publishing

Go into the `package.json` and give your package the desired name through the `"name"` option. Also consider adding a `"license"` field and point it to a `LICENSE` file which you can create from a template (one popular option is the [MIT license](https://opensource.org/license/mit/)).

To publish your library to [npm](https://www.npmjs.com):

```sh
npm publish
```

## Quick Command Reference

| Command | Explanations |
|---|---|
| `mise tasks` | List all available `mise` tasks in `shared/`. |
| `mise run autotest` | Run the AutoTester CLI package suite (`go/cmd/autotester`). |
| `TESTNAME=goose_pg mise run go-test` | Run all Go tests under `./go/...` with PostgreSQL DSN built from env vars in `mise.local.toml`, always passing `-testname`. |
| `TESTNAME=goose_pg TEST_DIR=./go/api/goose RECURSIVE=true mise run go-test` | Run tests for a specific directory and all its subdirectories with a required test name. |
| `TESTNAME=goose_pg TEST_DIR=./go/api/goose RECURSIVE=false mise run go-test` | Run tests only in one specific directory (no subdirectories) with a required test name. |
| `TESTNAME=goose_pg TEST_DIR=./go/api/goose RECURSIVE=false RUN_FILTER='^TestGoosePG' mise run go-test` | Run matching tests in a specific directory using `go test -run`, with a required test name. |
| `go test ./go/... -v -count=1 -args -testname goose_pg` | Run all Go tests in `shared/go` directly (without `mise`) while passing an explicit test name. |
| `go test ./go/api/goose -run '^TestGoosePG' -v -count=1 -args -testname goose_pg` | Run goose tests directly with verbose output, no cache, and explicit test name. |
--------
--------
