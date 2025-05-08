//go:build !solution

package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var numWorker = 1
var findEmail = regexp.MustCompile("<(.*)@(.*)>")

func GetPersonType(useCommiter bool) string {
	if useCommiter {
		return "committer"
	} else {
		return "author"
	}
}

func GetPersonTypeLog(useCommiter bool) string {
	if useCommiter {
		return "Commit"
	} else {
		return "Author"
	}
}

func ProccedPersonLog(personLine string, useCommiter bool) string {
	email := findEmail.FindString(personLine)
	person, okSuff := strings.CutSuffix(personLine, " "+email+"\n")
	person, okPref := strings.CutPrefix(person, GetPersonTypeLog(useCommiter)+": ")
	if okSuff && okPref {
		return person
	} else {
		return ""
	}
}

type PersonInfo struct {
	Name    string `json:"name"`
	Lines   int    `json:"lines"`
	Commits int    `json:"commits"`
	Files   int    `json:"files"`
}

type Language struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Extensions []string `json:"extensions"`
}

var (
	flagRepository  string
	flagRevision    string
	flagOrderBy     string
	flagUseCommiter bool
	flagFormat      string
	flagExtensions  []string
	flagLangs       []string
	flagExclude     []string
	flagRestrict    []string

	rootCmd = &cobra.Command{
		Use:   "gitfame",
		Short: "Gitfame calculates repository statistics",
		Long:  "Gitfame calculates repository statistics",
		Run: func(cmd *cobra.Command, args []string) {
			gitGetTree := exec.Command("/bin/git", "ls-tree", flagRevision, "-r", "--full-name", "--name-only")
			gitGetTree.Dir = flagRepository

			filePath := filepath.Join("..", "..", "configs", "language_extensions.json")
			data, err := os.ReadFile(filePath)
			if err != nil {
				fmt.Printf("Error reading file: %v\n", err)
				panic(err)
			}

			// Декодируем JSON
			var langs []Language
			if err4 := json.Unmarshal(data, &langs); err4 != nil {
				panic(err4)
			}

			output, err := gitGetTree.Output()
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}

			// Files, that should be processed:
			filesList := strings.Split(string(output[:]), "\n")

			// Some Concurrency staff:
			ticketer := make(chan struct{}, numWorker)

			var wg sync.WaitGroup
			var mu sync.Mutex

			linesCount := make(map[string]int)
			commitsCount := make(map[string]int)
			filesCount := make(map[string]int)
			commitSet := make(map[string]struct{})

			// Regex for parsing:
			var findPersonBlame = regexp.MustCompile(GetPersonType(flagUseCommiter) + "(.*)\n")
			var findPersonLog = regexp.MustCompile(GetPersonTypeLog(flagUseCommiter) + "(.)*\n")
			var findChangeLine = regexp.MustCompile("[0-9a-f]{40} [0-9]+ [0-9]+ [0-9]+\n")
			var findCommitSHA = regexp.MustCompile("[0-9a-f]{40}")

			// Supported languages:

			acceptableLangs := make(map[string]bool)
			acceptableExtension := make(map[string]bool)
			acceptableExtensionFlag := make(map[string]bool)

			for _, ext := range flagExtensions {
				acceptableExtensionFlag[ext] = true
			}

			for _, lang := range flagLangs {
				acceptableLangs[strings.ToLower(lang)] = true
			}

			for _, lang := range langs {
				if len(flagLangs) == 0 {
					break
				}

				if acceptableLangs[strings.ToLower(lang.Name)] {
					for _, ext := range lang.Extensions {
						acceptableExtension[ext] = true
					}
				}
			}

			for _, file := range filesList {
				if len(strings.Fields(file)) == 0 {
					continue
				}

				wg.Add(1)
				ticketer <- struct{}{}

				go func(fileName string) {
					defer wg.Done()

					haveExcludedPatterns := false
					haveRestrictPattern := (len(flagRestrict) == 0)

					for _, excludedPattern := range flagExclude {
						matched, ok := filepath.Match(excludedPattern, fileName)
						if ok != nil {
							fmt.Fprintln(os.Stderr, ok)
							os.Exit(1)
						}
						haveExcludedPatterns = haveExcludedPatterns || matched
					}

					for _, restrictPattern := range flagRestrict {
						matched, ok := filepath.Match(restrictPattern, fileName)
						if ok != nil {
							fmt.Fprintln(os.Stderr, ok)
							os.Exit(1)
						}
						haveRestrictPattern = haveRestrictPattern || matched
					}

					ext := path.Ext(fileName)
					acceptableExtLang := acceptableExtension[ext]
					acceptableExtLang = acceptableExtLang || (len(acceptableExtension) == 0)
					acceptableExtFlag := acceptableExtensionFlag[ext]
					acceptableExtFlag = acceptableExtFlag || (len(acceptableExtensionFlag) == 0)

					if haveExcludedPatterns || !haveRestrictPattern || !acceptableExtLang || !acceptableExtFlag {
						<-ticketer
						return
					}

					gitCountLines := exec.Command("/bin/git", "blame", "--incremental", flagRevision, "--", fileName)
					gitCountLines.Dir = flagRepository
					info, err := gitCountLines.Output()
					if err != nil {
						fmt.Fprintln(os.Stderr, err)
						os.Exit(1)
					}

					if len(strings.Fields(string(info[:]))) == 0 {
						getInitCommit := exec.Command("/bin/git", "log", "--pretty=full", flagRevision, "--", fileName)
						getInitCommit.Dir = flagRepository
						info1, err := getInitCommit.Output()
						if err != nil {
							fmt.Fprintln(os.Stderr, err)
							os.Exit(1)
						}

						textInfo := string(info1[:])
						commitSHA := findCommitSHA.FindString(textInfo)
						personLine := findPersonLog.FindString(textInfo)
						person := ProccedPersonLog(personLine, flagUseCommiter)
						if len(person) == 0 {
							<-ticketer
							return
						}

						mu.Lock()
						filesCount[person] += 1
						if _, ok := commitSet[commitSHA]; !ok {
							commitSet[commitSHA] = struct{}{}
							commitsCount[person] += 1
						}
						mu.Unlock()
						<-ticketer
						return
					}

					blocksWithInfo := strings.Split(string(info[:]), "filename")
					var fileMembers = make(map[string]struct{})
					var lastPerson string

					for _, blockInfo := range blocksWithInfo {
						if len(strings.Fields(blockInfo)) == 0 {
							continue
						}

						newPerson := findPersonBlame.MatchString(blockInfo)
						if newPerson {
							commiterInfo, _ := strings.CutSuffix(findPersonBlame.FindString(blockInfo), "\n")
							_, person, _ := strings.Cut(commiterInfo, " ")
							lastPerson = person
						}

						lineBorders, _ := strings.CutSuffix(findChangeLine.FindString(blockInfo), "\n")
						if len(lineBorders) == 0 {
							continue
						}

						borders := strings.Split(lineBorders, " ")
						commitSHA := borders[0]
						amount, _ := strconv.Atoi(borders[3])

						if len(lastPerson) == 0 {
							fmt.Fprintln(os.Stderr, "Something strange...")
							continue
						}

						mu.Lock()
						linesCount[lastPerson] += amount
						fileMembers[lastPerson] = struct{}{} // ento nado 1 raz delat!!!
						if _, ok := commitSet[commitSHA]; !ok {
							commitSet[commitSHA] = struct{}{}
							commitsCount[lastPerson] += 1
						}
						mu.Unlock()
					}

					mu.Lock()
					for person := range fileMembers {
						filesCount[person] += 1
					}
					mu.Unlock()

					<-ticketer
				}(file)

			}
			wg.Wait()

			finalInfo := make([]PersonInfo, 0)
			for person := range commitsCount {
				finalInfo = append(finalInfo, PersonInfo{person, linesCount[person], commitsCount[person], filesCount[person]})
			}

			if flagOrderBy == "lines" {
				sort.Slice(finalInfo, func(i, j int) bool {
					xi, xj := finalInfo[i], finalInfo[j]
					switch {
					case xi.Lines > xj.Lines:
						return true
					case xi.Lines == xj.Lines && xi.Commits > xj.Commits:
						return true
					case xi.Lines == xj.Lines && xi.Commits == xj.Commits && xi.Files > xj.Files:
						return true
					case xi.Lines == xj.Lines && xi.Commits == xj.Commits && xi.Files == xj.Files:
						return xi.Name < xj.Name
					default:
						return false
					}
				})
			} else if flagOrderBy == "commits" {
				sort.Slice(finalInfo, func(i, j int) bool {
					xi, xj := finalInfo[i], finalInfo[j]
					switch {
					case xi.Commits > xj.Commits:
						return true
					case xi.Commits == xj.Commits && xi.Lines > xj.Lines:
						return true
					case xi.Commits == xj.Commits && xi.Lines == xj.Lines && xi.Files > xj.Files:
						return true
					case xi.Lines == xj.Lines && xi.Commits == xj.Commits && xi.Files == xj.Files:
						return xi.Name < xj.Name
					default:
						return false
					}
				})
			} else if flagOrderBy == "files" {
				sort.Slice(finalInfo, func(i, j int) bool {
					xi, xj := finalInfo[i], finalInfo[j]
					switch {
					case xi.Files > xj.Files:
						return true
					case xi.Files == xj.Files && xi.Lines > xj.Lines:
						return true
					case xi.Files == xj.Files && xi.Lines == xj.Lines && xi.Commits > xj.Commits:
						return true
					case xi.Lines == xj.Lines && xi.Commits == xj.Commits && xi.Files == xj.Files:
						return xi.Name < xj.Name
					default:
						return false
					}
				})
			} else {
				fmt.Fprintln(os.Stderr, "Wrong order-by flag! Use --help to get usage inforamtion.")
				os.Exit(1)
			}

			if flagFormat == "tabular" {
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
				fmt.Fprintln(w, "Name\tLines\tCommits\tFiles")
				for _, line := range finalInfo {
					fmt.Fprintf(w, line.Name+"\t")
					fmt.Fprintf(w, strconv.Itoa(line.Lines)+"\t")
					fmt.Fprintf(w, strconv.Itoa(line.Commits)+"\t")
					fmt.Fprintf(w, strconv.Itoa(line.Files)+"\n")
				}
				w.Flush()
			} else if flagFormat == "csv" {
				w := csv.NewWriter(os.Stdout)
				err2 := w.Write([]string{"Name", "Lines", "Commits", "Files"})
				if err2 != nil {
					return
				}
				for _, line := range finalInfo {
					lineData := make([]string, 0)
					lineData = append(lineData, line.Name)
					lineData = append(lineData, strconv.Itoa(line.Lines))
					lineData = append(lineData, strconv.Itoa(line.Commits))
					lineData = append(lineData, strconv.Itoa(line.Files))
					err3 := w.Write(lineData)
					if err3 != nil {
						return
					}
				}
				w.Flush()
			} else if flagFormat == "json" || flagFormat == "json-lines" {
				var separtor string
				if flagFormat == "json" {
					separtor = ","
				} else {
					separtor = "\n"
				}

				if flagFormat == "json" {
					fmt.Fprint(os.Stdout, "[")
				}

				for j, line := range finalInfo {
					encoded, err := json.Marshal(line)
					if err != nil {
						fmt.Fprintln(os.Stderr, err)
						os.Exit(1)
					}
					fmt.Fprint(os.Stdout, string(encoded[:]))
					if j != (len(finalInfo)-1) || flagFormat == "json-lines" {
						fmt.Fprint(os.Stdout, separtor)
					}
				}

				if flagFormat == "json" {
					fmt.Fprint(os.Stdout, "]\n")
				}
			} else {
				fmt.Fprintln(os.Stderr, "Wrong format flag! Use --help to get usage inforamtion.")
				os.Exit(1)
			}

		},
	}
)

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	rootCmd.Flags().StringVar(&flagRepository, "repository", dir, "Path to GIT repository, curr")
	rootCmd.Flags().StringVar(&flagRevision, "revision", "HEAD", "Commit pointer")
	rootCmd.Flags().StringVar(&flagOrderBy, "order-by", "lines", "Type of sort applies to result")
	rootCmd.Flags().BoolVar(&flagUseCommiter, "use-committer", false, "Replace author to commiter")
	rootCmd.Flags().StringVar(&flagFormat, "format", "tabular", "Type of output")
	rootCmd.Flags().StringSliceVar(&flagExtensions, "extensions", []string{}, "Extensions considered")
	rootCmd.Flags().StringSliceVar(&flagLangs, "languages", []string{}, "Accepted laguages")
	rootCmd.Flags().StringSliceVar(&flagExclude, "exclude", []string{}, "Excluded Glob patterns")
	rootCmd.Flags().StringSliceVar(&flagRestrict, "restrict-to", []string{}, "Patterns that at least one must be satisfied")
}

func main() {
	Execute()
}
