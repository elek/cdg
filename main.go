package main

import (
	"fmt"
	"github.com/alecthomas/kong"
	"github.com/go-delve/delve/pkg/proc"
	"github.com/go-delve/delve/pkg/proc/core"
	"log"
	"net"
	"slices"
	"strconv"
	"strings"
)

type cli struct {
	Executable    string `arg:""`
	CoreDump      string `arg:""`
	ShowVariables bool
	ID            int64
	Frame         int
	Depth         int `default:"5"`
	Variable      string
	Expr          string
	FrameFilter   string
	Filter        string
	Limit         int
}

func main() {
	ctx := kong.Parse(&cli{})
	err := ctx.Run()
	if err != nil {
		log.Fatal(err)
	}
}

func (c *cli) Run() error {
	oc, err := core.OpenCore(c.CoreDump, c.Executable, []string{})
	if err != nil {
		panic(err)
	}

	target := oc.Targets()[0]
	info, _, err := proc.GoroutinesInfo(target, 0, 100000)
	if err != nil {
		panic(err)
	}
	fmt.Println(len(info))
	slices.SortFunc[[]*proc.G](info, func(a *proc.G, b *proc.G) int {
		if a.WaitSince < b.WaitSince {
			return -1
		}
		return 1
	})
	count := 0
	for _, g := range info {
		if c.ID > 0 && g.ID != c.ID {
			continue
		}
		stacktrace, err := proc.GoroutineStacktrace(target, g, 40, 0)

		if c.Filter != "" {
			match := false
			for _, frame := range stacktrace {
				if strings.Contains(frame.Call.Fn.Name, c.Filter) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		count++
		if c.Limit > 0 && count > c.Limit {
			break
		}
		fmt.Println("goroutine", g.ID, waitReasonStrings[g.WaitReason], (g.WaitSince-24618267275634)/1000000000)
		fmt.Println("--> go", loc(g.Go()))
		for k, v := range g.Labels() {
			fmt.Println("    ", k, v)
		}
		if err != nil {
			panic(err)
		}

		cfg := proc.LoadConfig{
			FollowPointers:     true,
			MaxVariableRecurse: c.Depth + 1,
			MaxStringLen:       10000,
			MaxStructFields:    100,
			MaxArrayValues:     100,
		}
		for ix, frame := range stacktrace {
			if c.Frame > 0 && ix != c.Frame {
				continue
			}
			if c.FrameFilter != "" && !strings.Contains(frame.Call.Fn.Name, c.FrameFilter) {
				continue
			}
			fmt.Println(ix, loc(frame.Call))
			if c.ShowVariables {
				scope := proc.FrameToScope(target, target.Memory(), nil, 0, stacktrace[ix:]...)
				locals, err := scope.LocalVariables(cfg)
				if err != nil {
					panic(err)
				}
				args, err := scope.FunctionArguments(cfg)
				if err != nil {
					panic(err)
				}

				for k, v := range map[string][]*proc.Variable{"A": args, "L": locals} {
					for _, l := range v {
						if c.Variable == "" || strings.Contains(l.Name, c.Variable) {
							if c.Expr != "" {
								expression, err := scope.EvalExpression(l.Name+"."+c.Expr, cfg)
								if err != nil {
									fmt.Println(err)
									continue
								}
								printVariable(*expression, " "+k, "  ", c.Depth)
							} else {
								printVariable(*l, " "+k, "  ", c.Depth)
							}
						}

					}
				}
			}
		}
		fmt.Println()
	}
	return nil
}

func printVariable(l proc.Variable, prefix string, indent string, depth int) {
	if depth == 0 {
		return
	}
	value := ""
	if l.Value != nil {
		value = l.Value.String()
	}
	skipChildren := false
	if l.TypeString() == "net.IP" {
		ip := net.IP{}
		for _, c := range l.Children {
			i, _ := strconv.Atoi(c.Value.String())
			ip = append(ip, byte(i))
		}
		value = ip.String()
		skipChildren = true

	}
	fmt.Printf("%s %s: %s (%s) %s\n", prefix, l.Name, value, l.TypeString(), l.Kind)
	if !skipChildren {
		for _, c := range l.Children {
			printVariable(c, prefix+indent, indent, depth-1)
		}
	}
}

func loc(location proc.Location) string {
	return fmt.Sprintf("%s:%d %s", location.File, location.Line, location.Fn.Name)
}

var waitReasonStrings = [...]string{
	"",
	"GC assist marking",
	"IO wait",
	"chan receive (nil chan)",
	"chan send (nil chan)",
	"dumping heap",
	"garbage collection",
	"garbage collection scan",
	"panicwait",
	"select",
	"select (no cases)",
	"GC assist wait",
	"GC sweep wait",
	"GC scavenge wait",
	"chan receive",
	"chan send",
	"finalizer wait",
	"force gc (idle)",
	"semacquire",
	"sleep",
	"sync.Cond.Wait",
	"timer goroutine (idle)",
	"trace reader (blocked)",
	"wait for GC cycle",
	"GC worker (idle)",
	"preempted",
	"debug call",
}
