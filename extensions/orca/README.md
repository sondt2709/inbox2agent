# i2a plugin for Orca

This plugin shows the notifications of i2a in Orca. Orca also sends them to the phones that are paired with Orca. i2a calls the plugin command `notify` through the Orca runtime socket (`plugins.invokeCommand`). The plugin needs one capability: `notifications:show`.

The plugin system of Orca is experimental. Orca 1.4.240 has it, and it is off by default.

## Install

1. Run `i2a notify setup`. It writes this folder to `<data>/extensions/orca/` and prints the path.
2. In Orca, open **Settings → Plugins** and turn on the plugin system.
3. Install a plugin from a local folder, and select the folder from step 1.
4. Allow the capability `notifications:show`.
5. Run `i2a notify test`. It prints `orca` when Orca showed the test note.

With `[notify] via = "auto"` (the default), i2a uses Orca first, then terminal-notifier, then `osascript`. Orca shows each note as `i2a.notify: <title>` (the plugin key comes first).
