# Renders assets/images/overview.png. Run from the repository root after "make build".
Output assets/images/demo.gif

Set FontSize 14
Set Width 1400
Set Height 800
Set Theme "Catppuccin Mocha"

# The demo lives in a throwaway home. The prompt only comes back once the
# script has created the ready marker; if the script fails it never does, and
# the Wait times out instead of taking screenshots of a broken workspace.
Hide
Type@5ms `unset DEMO_HOME; export PS1='$ ' PATH="$PWD/bin:$PATH"`
Enter
Type@5ms `DEMO_HOME=$(mktemp -d) && [ -d "$DEMO_HOME" ] && DEMO_HOME=$(cd "$DEMO_HOME" && pwd -P) && touch "$DEMO_HOME/.gitdrift-demo-home" && export HOME="$DEMO_HOME"`
Enter
Type@5ms `[ -f "$DEMO_HOME/.gitdrift-demo-home" ] && scripts/demo-workspace.sh "$DEMO_HOME/code" > /dev/null && touch "$DEMO_HOME/ready"; until [ -e "$DEMO_HOME/ready" ]; do sleep 1; done; cd "$DEMO_HOME/code/work" && clear`
Enter
Wait@120s /^\$$/
Show

Type "gdrift"
Enter
Sleep 5s
Type "j"
Sleep 2s
Screenshot assets/images/overview.png
# Let the capture land before the next key changes the frame.
Sleep 1s

# Tour: commits, files and diff, the Branches tab, then the zoomed table.
Type "2"
Sleep 2s
Type "j"
Sleep 2s
Type "j"
Sleep 2s
Type "]"
Type "]"
Type "]"
Sleep 3s
Type "1"
Sleep 2s
Type "z"
Sleep 3s
Type "z"
Sleep 2s

# Quit gdrift and remove the throwaway home, never anything else.
Hide
Type "q"
Wait /^\$$/
Type@5ms `[ -n "$DEMO_HOME" ] && [ "$DEMO_HOME" != "$PWD" ] && [ -f "$DEMO_HOME/.gitdrift-demo-home" ] && rm -rf "$DEMO_HOME"`
Enter
Wait /^\$$/
