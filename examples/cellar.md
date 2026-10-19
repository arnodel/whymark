---
title: The Cellar
description: A tiny adventure in two rooms, the whymark format's running example.
whymark:
  version: 1
  journal: true
---

A tiny adventure: find the lamp, and go down to the cellar. Open it in whymark to play
(`whymark examples/cellar.md`), or read it here: each `#` heading is a passage, and
the code in backquotes runs. See [whymark.md](../whymark.md) for the format.

```lua
function init(turn)
  return { lamp = false }
end
```

# Hall

You're in a hall. `= turn.state.lamp and "Your lamp glows." or "Stairs go down into the dark."`

`if not turn.state.lamp then`
- [Take the lamp](#take-the-lamp)
`end`
- [Go down to the cellar](#cellar)

# (Take the lamp)

```lua
turn.state.lamp = true
turn.write_headers{summary = "Took the lamp"}
```

You take the lamp from its hook. [Back to the hall](#hall)

# Cellar

```lua
if turn.state.lamp then
  turn.write_headers{summary = "Found the port", mark = "green"}
else
  turn.write_headers{summary = "Stumbled in the dark", mark = "red"}
end
```

`if turn.state.lamp then`
Your lamp lights up a dusty bottle of 1887 port. **You win!** [Play again](cellar.md)
`else`
It's pitch black. You stumble, and climb back up. [Back to the hall](#hall)
`end`
