# VERSIONS — frontend toolchain (discovered via `npm view`, 2026-08-13)

Source of truth for the pinned lines used across the pnpm workspace. All
versions below were queried live from the npm registry on 2026-08-13.

## Expo SDK 54 compatibility note

`expo@54.0.36` is the current patch of the SDK 54 line. Expo SDK 54 requires
**react-native 0.81.x** and **react 19.1.x** (verified lines on npm:
`react-native@0.81.6`, `react@19.1.9`); its own `peerDependencies` on
react/react-native are `*`, so the pin comes from the SDK's bundled native
modules, not from npm peer ranges — mobile MUST stay on the 0.81/19.1 lines.
The web app (Vite, no Expo constraint) can use react/react-dom 19.2.x.

SDK-54-compatible Expo library lines (latest patch of each line queried):
`expo-router@~6.0` (6.0.24), `expo-notifications@~0.32` (0.32.17),
`expo-av@~16.0` (16.0.8) and `expo-audio@~1.0` (1.0.16). In SDK 54
**expo-audio is the current audio API** (expo-av is legacy/deprecated); we
record both, mobile should use `expo-audio` for playback.

TypeScript: `latest` on npm is now **7.0.2** (the native compiler line);
the stable ecosystem line used by Expo SDK 54 and current Vite tooling is
**5.9.3**, which is what the workspace pins (`~5.9.3`).

## Discovered versions

| Package | Version (npm view, 2026-08-13) | Note |
|---|---|---|
| expo | 54.0.36 | SDK 54 line (`~54.0.0`) |
| react | 19.2.8 | latest — web only |
| react (mobile) | 19.1.9 | line required by Expo SDK 54 (`~19.1.0`) |
| react-dom | 19.2.8 | web |
| react-native | 0.81.6 | line required by Expo SDK 54 (`~0.81.0`) |
| vite | 8.2.1 | |
| @vitejs/plugin-react | 6.0.5 | |
| typescript | 7.0.2 (latest) / 5.9.3 (pinned) | workspace pins `~5.9.3`, see note |
| @tanstack/react-router | 1.170.27 | |
| @tanstack/react-query | 5.101.4 | |
| @tanstack/react-query-devtools | 5.101.4 | |
| tailwindcss | 4.3.3 | |
| zod | 4.4.3 | zod v4 API |
| expo-router | 6.0.24 | SDK 54 line (`~6.0.0`); npm `latest` is 57.x (SDK 57) |
| expo-notifications | 0.32.17 | SDK 54 line (`~0.32.0`); npm `latest` is 57.x |
| expo-av | 16.0.8 | SDK 54 line — legacy, prefer expo-audio |
| expo-audio | 1.0.16 | SDK 54 line (`~1.0.0`); current audio API |
| i18next | 26.3.6 | |
| react-i18next | 17.0.11 | |
| pnpm | 11.21.0 | `packageManager` pin |

> Expo libraries recently switched `latest` to SDK-aligned major versions
> (e.g. `expo-router@57.0.12` targets SDK 57). Always install Expo libraries
> with `npx expo install`, which resolves the SDK-54-compatible line above.
