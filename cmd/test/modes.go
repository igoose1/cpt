package test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/cp-tools/cpt/util"

	"github.com/fatih/color"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

func judgeMode(runScript, checkerTmplt string, timelimit time.Duration,
	inputFile, expectedFile string, index int) {
	// Verdict template configurations.
	var verdictData struct {
		Index      int
		Verdict    string
		Elapsed    time.Duration
		FailLog    error
		Stderr     string
		CheckerLog string
		Compare    string
	}

	// handle panic; print verdict.
	defer func() {
		verdictData.Index = index
		// handle panic; recover.
		if r := recover(); r != nil {
			verdictData.Verdict = color.RedString("FAIL")
			verdictData.FailLog = r.(error)
		}

		// Verdict format is as follows:

		c := color.New(color.FgBlue, color.Bold).SprintFunc()
		out, _ := util.CleanTemplate(strings.Join([]string{
			// Test: #4    Verdict: WA    Time: 32ms
			c("Test:") + " #{{.Index}}" + "\t" + c("Verdict:") + " {{.Verdict}}" + "\t" + c("Time:") + " {{.Elapsed}}",
			// Fail: Could not execute checker
			"{{- if .FailLog}}\n" + c("Fail:") + " {{.FailLog}}" + "{{end}}",
			// Stderr:
			// 1 2 3
			// a b c
			"{{- if .Stderr}}\n" + c("Stderr:") + "\n{{.Stderr}}" + "{{end}}",
			// Checker Log: Wrong answer, expected 3, found 4.
			"{{- if .CheckerLog}}\n" + c("Checker Log:") + " {{.CheckerLog}}" + "{{end}}",
			// INPUT     | EXPECTED | OUTPUT
			// 5 3       | 3        | 4
			// 1 2 3 4 5 | 1        | 1
			"{{- if .Compare}}\n" + "{{.Compare}}" + "{{end}}",
		}, "\n"), verdictData)

		// Print box around verdict
		t := table.NewWriter()
		t.SetStyle(table.StyleBold)
		t.AppendRow(table.Row{strings.TrimSpace(out)})

		fmt.Println(t.Render())
	}()

	// Read input from file.
	input, err := os.Open(inputFile)
	if err != nil {
		panic(err)
	}
	defer input.Close()

	// Run code against test case.
	var output, stderr bytes.Buffer
	elapsed, err := runShellScript(runScript, timelimit, input, &output, &stderr)

	verdictData.Elapsed = elapsed.Truncate(time.Millisecond)
	verdictData.Stderr = stderr.String()

	// Determine verdicts.
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		// It's a time limit exceeded case.
		verdictData.Verdict = color.YellowString("TLE")

	case err != nil:
		// It's a runtime error case.
		verdictData.Verdict = color.RedString("RTE")

	default:
		// Output recieved, and program exited normally.
		// Check if output matches required answer(s).

		// write output to temporary file.
		outputFile, err := ioutil.TempFile(os.TempDir(), "cpt-test-output-")
		if err != nil {
			panic(err)
		}
		defer os.Remove(outputFile.Name())
		outputFile.Write(output.Bytes())

		// Run checker to validate output.
		checkerScript, err := util.CleanTemplate(checkerTmplt, map[string]string{
			"inputFile":    inputFile,
			"outputFile":   outputFile.Name(),
			"expectedFile": expectedFile,
		})
		if err != nil {
			panic(err)
		}

		var checkerStderr bytes.Buffer
		_, err = runShellScript(checkerScript, time.Minute, nil, nil, &checkerStderr)

		verdictData.CheckerLog = checkerStderr.String()

		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
			// Checker ended with error code 1.
			// Verdict is thus wrong answer.
			verdictData.Verdict = color.RedString("WA")

			// Read input from file.
			input, err := os.Open(inputFile)
			if err != nil {
				panic(err)
			}
			defer input.Close()

			inputBuf := make([]byte, 80)
			if n, _ := input.Read(inputBuf); n == len(inputBuf) {
				inputBuf = append(inputBuf[:n-3], []byte("...")...)
			}
			inputBuf = bytes.Trim(inputBuf, "\x00")

			// Read expected from file.
			expected, err := os.Open(expectedFile)
			if err != nil {
				panic(err)
			}
			defer expected.Close()

			expectedBuf := make([]byte, 80)
			if n, _ := expected.Read(expectedBuf); n == len(expectedBuf) {
				expectedBuf = append(expectedBuf[:n-3], []byte("...")...)
			}
			expectedBuf = bytes.Trim(expectedBuf, "\x00")

			// Read output from created output file.
			output, err := os.Open(outputFile.Name())
			if err != nil {
				panic(err)
			}
			defer output.Close()

			outputBuf := make([]byte, 80)
			if n, _ := output.Read(outputBuf); n == len(outputBuf) {
				outputBuf = append(outputBuf[:n-3], []byte("...")...)
			}
			outputBuf = bytes.Trim(outputBuf, "\x00")

			// Table to display output difference.
			t := table.NewWriter()
			t.SetStyle(table.StyleLight)
			t.Style().Options.DrawBorder = false

			headerColor := text.Colors{text.FgBlue, text.Bold}
			t.SetColumnConfigs([]table.ColumnConfig{
				{Number: 1, AlignHeader: text.AlignCenter, ColorsHeader: headerColor, WidthMax: 40},
				{Number: 2, AlignHeader: text.AlignCenter, ColorsHeader: headerColor, WidthMax: 40},
				{Number: 3, AlignHeader: text.AlignCenter, ColorsHeader: headerColor, WidthMax: 40},
			})

			t.AppendHeader(table.Row{"INPUT", "EXPECTED", "OUTPUT"})
			t.AppendRow(table.Row{string(inputBuf), string(expectedBuf), string(outputBuf)})

			verdictData.Compare = t.Render()

		} else if err != nil {
			// Unknown error; Panic.
			panic(err)
		} else {
			// Solution produced correct answer.
			verdictData.Verdict = color.GreenString("AC")
		}
	}
}

func interactiveMode(script string) {
	// It doesn't get any simpler, does it?
	fmt.Println(color.GreenString("---- * ---- launched ---- * ----"))
	runShellScript(script, time.Hour, os.Stdin, os.Stdout, os.Stderr)
	fmt.Println(color.GreenString("---- * ---- finished ---- * ----"))
}
