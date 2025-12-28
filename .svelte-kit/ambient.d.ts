
// this file is generated — do not edit it


/// <reference types="@sveltejs/kit" />

/**
 * Environment variables [loaded by Vite](https://vitejs.dev/guide/env-and-mode.html#env-files) from `.env` files and `process.env`. Like [`$env/dynamic/private`](https://svelte.dev/docs/kit/$env-dynamic-private), this module cannot be imported into client-side code. This module only includes variables that _do not_ begin with [`config.kit.env.publicPrefix`](https://svelte.dev/docs/kit/configuration#env) _and do_ start with [`config.kit.env.privatePrefix`](https://svelte.dev/docs/kit/configuration#env) (if configured).
 * 
 * _Unlike_ [`$env/dynamic/private`](https://svelte.dev/docs/kit/$env-dynamic-private), the values exported from this module are statically injected into your bundle at build time, enabling optimisations like dead code elimination.
 * 
 * ```ts
 * import { API_KEY } from '$env/static/private';
 * ```
 * 
 * Note that all environment variables referenced in your code should be declared (for example in an `.env` file), even if they don't have a value until the app is deployed:
 * 
 * ```
 * MY_FEATURE_FLAG=""
 * ```
 * 
 * You can override `.env` values from the command line like so:
 * 
 * ```sh
 * MY_FEATURE_FLAG="enabled" npm run dev
 * ```
 */
declare module '$env/static/private' {
	export const SHELL: string;
	export const LSCOLORS: string;
	export const npm_command: string;
	export const GHOSTTY_BIN_DIR: string;
	export const COLORTERM: string;
	export const __HM_SESS_VARS_SOURCED: string;
	export const XDG_CONFIG_DIRS: string;
	export const LESS: string;
	export const XPC_FLAGS: string;
	export const TERM_PROGRAM_VERSION: string;
	export const GITHUB_OAUTH_CLIENT_ID: string;
	export const TMUX: string;
	export const _P9K_TTY: string;
	export const NODE: string;
	export const __CFBundleIdentifier: string;
	export const SSH_AUTH_SOCK: string;
	export const P9K_TTY: string;
	export const npm_config_local_prefix: string;
	export const HOMEBREW_PREFIX: string;
	export const EDITOR: string;
	export const PWD: string;
	export const NIX_PROFILES: string;
	export const LOGNAME: string;
	export const MANPATH: string;
	export const LaunchInstanceID: string;
	export const GOOGLE_CLIENT_SECRET: string;
	export const __NIX_DARWIN_SET_ENVIRONMENT_DONE: string;
	export const _: string;
	export const ZSH_TMUX_CONFIG: string;
	export const COMMAND_MODE: string;
	export const GHOSTTY_SHELL_FEATURES: string;
	export const HOME: string;
	export const LANG: string;
	export const LS_COLORS: string;
	export const npm_package_version: string;
	export const _ZSH_TMUX_FIXED_CONFIG: string;
	export const SECURITYSESSIONID: string;
	export const NIX_SSL_CERT_FILE: string;
	export const TMPDIR: string;
	export const NIX_USER_PROFILE_DIR: string;
	export const INFOPATH: string;
	export const npm_lifecycle_script: string;
	export const GHOSTTY_RESOURCES_DIR: string;
	export const TERM: string;
	export const TERMINFO: string;
	export const npm_package_name: string;
	export const USER: string;
	export const TMUX_PANE: string;
	export const HOMEBREW_CELLAR: string;
	export const npm_lifecycle_event: string;
	export const SHLVL: string;
	export const PAGER: string;
	export const __HM_ZSH_SESS_VARS_SOURCED: string;
	export const HOMEBREW_REPOSITORY: string;
	export const _P9K_SSH_TTY: string;
	export const ATUIN_SESSION: string;
	export const XPC_SERVICE_NAME: string;
	export const npm_config_user_agent: string;
	export const TERMINFO_DIRS: string;
	export const npm_execpath: string;
	export const VI_MODE_SET_CURSOR: string;
	export const ATUIN_HISTORY_ID: string;
	export const ZSH_TMUX_TERM: string;
	export const DOCKER_HOST: string;
	export const npm_package_json: string;
	export const P9K_SSH: string;
	export const GITHUB_OAUTH_CLIENT_SECRET: string;
	export const XDG_DATA_DIRS: string;
	export const GOOGLE_OAUTH_CLIENT_ID: string;
	export const PATH: string;
	export const npm_node_execpath: string;
	export const OLDPWD: string;
	export const __CF_USER_TEXT_ENCODING: string;
	export const TERM_PROGRAM: string;
}

/**
 * Similar to [`$env/static/private`](https://svelte.dev/docs/kit/$env-static-private), except that it only includes environment variables that begin with [`config.kit.env.publicPrefix`](https://svelte.dev/docs/kit/configuration#env) (which defaults to `PUBLIC_`), and can therefore safely be exposed to client-side code.
 * 
 * Values are replaced statically at build time.
 * 
 * ```ts
 * import { PUBLIC_BASE_URL } from '$env/static/public';
 * ```
 */
declare module '$env/static/public' {
	
}

/**
 * This module provides access to runtime environment variables, as defined by the platform you're running on. For example if you're using [`adapter-node`](https://github.com/sveltejs/kit/tree/main/packages/adapter-node) (or running [`vite preview`](https://svelte.dev/docs/kit/cli)), this is equivalent to `process.env`. This module only includes variables that _do not_ begin with [`config.kit.env.publicPrefix`](https://svelte.dev/docs/kit/configuration#env) _and do_ start with [`config.kit.env.privatePrefix`](https://svelte.dev/docs/kit/configuration#env) (if configured).
 * 
 * This module cannot be imported into client-side code.
 * 
 * ```ts
 * import { env } from '$env/dynamic/private';
 * console.log(env.DEPLOYMENT_SPECIFIC_VARIABLE);
 * ```
 * 
 * > [!NOTE] In `dev`, `$env/dynamic` always includes environment variables from `.env`. In `prod`, this behavior will depend on your adapter.
 */
declare module '$env/dynamic/private' {
	export const env: {
		SHELL: string;
		LSCOLORS: string;
		npm_command: string;
		GHOSTTY_BIN_DIR: string;
		COLORTERM: string;
		__HM_SESS_VARS_SOURCED: string;
		XDG_CONFIG_DIRS: string;
		LESS: string;
		XPC_FLAGS: string;
		TERM_PROGRAM_VERSION: string;
		GITHUB_OAUTH_CLIENT_ID: string;
		TMUX: string;
		_P9K_TTY: string;
		NODE: string;
		__CFBundleIdentifier: string;
		SSH_AUTH_SOCK: string;
		P9K_TTY: string;
		npm_config_local_prefix: string;
		HOMEBREW_PREFIX: string;
		EDITOR: string;
		PWD: string;
		NIX_PROFILES: string;
		LOGNAME: string;
		MANPATH: string;
		LaunchInstanceID: string;
		GOOGLE_CLIENT_SECRET: string;
		__NIX_DARWIN_SET_ENVIRONMENT_DONE: string;
		_: string;
		ZSH_TMUX_CONFIG: string;
		COMMAND_MODE: string;
		GHOSTTY_SHELL_FEATURES: string;
		HOME: string;
		LANG: string;
		LS_COLORS: string;
		npm_package_version: string;
		_ZSH_TMUX_FIXED_CONFIG: string;
		SECURITYSESSIONID: string;
		NIX_SSL_CERT_FILE: string;
		TMPDIR: string;
		NIX_USER_PROFILE_DIR: string;
		INFOPATH: string;
		npm_lifecycle_script: string;
		GHOSTTY_RESOURCES_DIR: string;
		TERM: string;
		TERMINFO: string;
		npm_package_name: string;
		USER: string;
		TMUX_PANE: string;
		HOMEBREW_CELLAR: string;
		npm_lifecycle_event: string;
		SHLVL: string;
		PAGER: string;
		__HM_ZSH_SESS_VARS_SOURCED: string;
		HOMEBREW_REPOSITORY: string;
		_P9K_SSH_TTY: string;
		ATUIN_SESSION: string;
		XPC_SERVICE_NAME: string;
		npm_config_user_agent: string;
		TERMINFO_DIRS: string;
		npm_execpath: string;
		VI_MODE_SET_CURSOR: string;
		ATUIN_HISTORY_ID: string;
		ZSH_TMUX_TERM: string;
		DOCKER_HOST: string;
		npm_package_json: string;
		P9K_SSH: string;
		GITHUB_OAUTH_CLIENT_SECRET: string;
		XDG_DATA_DIRS: string;
		GOOGLE_OAUTH_CLIENT_ID: string;
		PATH: string;
		npm_node_execpath: string;
		OLDPWD: string;
		__CF_USER_TEXT_ENCODING: string;
		TERM_PROGRAM: string;
		[key: `PUBLIC_${string}`]: undefined;
		[key: `${string}`]: string | undefined;
	}
}

/**
 * Similar to [`$env/dynamic/private`](https://svelte.dev/docs/kit/$env-dynamic-private), but only includes variables that begin with [`config.kit.env.publicPrefix`](https://svelte.dev/docs/kit/configuration#env) (which defaults to `PUBLIC_`), and can therefore safely be exposed to client-side code.
 * 
 * Note that public dynamic environment variables must all be sent from the server to the client, causing larger network requests — when possible, use `$env/static/public` instead.
 * 
 * ```ts
 * import { env } from '$env/dynamic/public';
 * console.log(env.PUBLIC_DEPLOYMENT_SPECIFIC_VARIABLE);
 * ```
 */
declare module '$env/dynamic/public' {
	export const env: {
		[key: `PUBLIC_${string}`]: string | undefined;
	}
}
