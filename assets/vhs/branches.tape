# Renders assets/images/branches.png. Run from the repository root after "make build".
Set FontSize 14
Set Width 1900
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
Sleep 1s
Type "2"
Type "]"
Type "]"
Type "]"
Sleep 1s
Screenshot assets/images/branches.png
Sleep 1s

# Quit gdrift and remove the throwaway home, never anything else.
Hide
Type "q"
Wait /^\$$/
Type@5ms `[ -n "$DEMO_HOME" ] && [ "$DEMO_HOME" != "$PWD" ] && [ -f "$DEMO_HOME/.gitdrift-demo-home" ] && rm -rf "$DEMO_HOME"`
Enter
Wait /^\$$/
