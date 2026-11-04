package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

const (
	minSteps = 20
	maxSteps = 1000000
	rows     = 10
)

type param struct {
	key   string
	title string
	unit  string
	hint  string
	def   float64
	min   float64
	max   float64
	low   string
	high  string
	whole bool
	value float64
}

func newParams() []*param {
	return []*param{
		{key: "n", title: "Число восприимчивых людей в сообществе (без заражённого)", unit: "чел.", def: 1000, min: 1, max: 1e6, whole: true,
			low:  "в сообществе должен быть хотя бы один восприимчивый человек",
			high: "программа рассчитана на сообщества не больше миллиона человек"},
		{key: "r", title: "Частота контактов r", unit: "1/(чел·сут)", def: 0.0005, min: 1e-12, max: 10,
			low:  "частота контактов должна быть больше нуля, иначе болезнь не передаётся",
			high: "при такой частоте контактов заражение происходит почти мгновенно"},
		{key: "step", title: "Шаг численного метода Рунге-Кутты", unit: "сут", def: 0.1, min: 1e-6, max: 100,
			low:  "с таким маленьким шагом счёт шёл бы слишком долго",
			high: "с таким большим шагом результат получается слишком грубым"},
	}
}

func (p *param) check(x float64) error {
	if x < p.min {
		return fmt.Errorf("значение должно быть от %s до %s %s: %s", fg(p.min), fg(p.max), p.unit, p.low)
	}
	if x > p.max {
		return fmt.Errorf("значение должно быть от %s до %s %s: %s", fg(p.min), fg(p.max), p.unit, p.high)
	}
	if p.whole && x != math.Trunc(x) {
		return errors.New("число людей должно быть целым")
	}
	return nil
}

func (p *param) caption() string {
	s := p.title + ", " + p.unit
	if p.hint != "" {
		s += " (" + p.hint + ")"
	}
	return s
}

var (
	numRe    = regexp.MustCompile(`^[+-]?(\d+([.,]\d*)?|[.,]\d+)([eE][+-]?\d+)?$`)
	leadRe   = regexp.MustCompile(`^[+-]?(\d+([.,]\d*)?|[.,]\d+)`)
	errEmpty = errors.New("значение не указано")
	errHelp  = errors.New("help")
)

func short(s string) string {
	r := []rune(s)
	if len(r) > 25 {
		return string(r[:25]) + "..."
	}
	return s
}

func parseNumber(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errEmpty
	}
	if !numRe.MatchString(s) {
		if strings.Count(s, ".")+strings.Count(s, ",") > 1 {
			return 0, fmt.Errorf("«%s» - в числе может быть только одна точка или запятая", short(s))
		}
		if lead := leadRe.FindString(s); lead != "" {
			rest := strings.TrimSpace(s[len(lead):])
			if strings.IndexFunc(rest, unicode.IsLetter) >= 0 || strings.ContainsAny(rest, "/°") {
				return 0, fmt.Errorf("«%s» - единицы измерения писать не нужно, введите только число", short(s))
			}
			if rest != "" && unicode.IsDigit([]rune(rest)[0]) {
				return 0, fmt.Errorf("«%s» - уберите пробелы внутри числа", short(s))
			}
		}
		return 0, fmt.Errorf("«%s» - это не число (пример правильного ввода: 0,0005 или 0.0005)", short(s))
	}
	x, err := strconv.ParseFloat(strings.Replace(s, ",", ".", 1), 64)
	if err != nil || math.IsInf(x, 0) || math.IsNaN(x) {
		return 0, fmt.Errorf("«%s» - слишком большое число", short(s))
	}
	return x, nil
}

func ff(x float64, dec int) string {
	s := strconv.FormatFloat(x, 'f', dec, 64)
	if strings.Trim(s, "-0.") == "" {
		s = strings.TrimPrefix(s, "-")
	}
	return strings.Replace(s, ".", ",", 1)
}

func fg(x float64) string {
	return strings.Replace(strconv.FormatFloat(x, 'f', -1, 64), ".", ",", 1)
}

func fe(x float64) string {
	if x == 0 {
		return "0"
	}
	return strings.Replace(strconv.FormatFloat(x, 'e', 1, 64), ".", ",", 1)
}

func fn(x float64, sig int) string {
	d := sig - 1
	if x != 0 {
		d = sig - 1 - int(math.Floor(math.Log10(math.Abs(x))))
	}
	if d < 0 {
		d = 0
	}
	if d > 12 {
		d = 12
	}
	s := ff(x, d)
	if strings.Contains(s, ",") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ",")
	}
	return s
}

type model struct {
	n, r float64
}

func (md model) rate() float64 {
	return (md.n + 1) * md.r
}

func (md model) exact(t float64) float64 {
	return (md.n + 1) / (1 + math.Exp(md.rate()*t-math.Log(md.n)))
}

func (md model) f(x float64) float64 {
	return -md.r * x * (md.n + 1 - x)
}

func (md model) step(x, dt float64) float64 {
	k1 := md.f(x)
	k2 := md.f(x + dt/2*k1)
	k3 := md.f(x + dt/2*k2)
	k4 := md.f(x + dt*k3)
	return x + dt/6*(k1+2*k2+2*k3+k4)
}

func (md model) advance(x, dur, dt float64) float64 {
	n := int(math.Ceil(dur/dt - 1e-9))
	d := dur / float64(n)
	for i := 0; i < n; i++ {
		x = md.step(x, d)
	}
	return x
}

func (md model) peak() float64 {
	return math.Log(md.n) / md.rate()
}

func (md model) end() float64 {
	return math.Log(99*md.n+100) / md.rate()
}

func (md model) maxError(span, dt float64) float64 {
	n := int(math.Ceil(span/dt - 1e-9))
	d := span / float64(n)
	x, e := md.n, 0.0
	for i := 1; i <= n; i++ {
		x = md.step(x, d)
		e = math.Max(e, math.Abs(x-md.exact(float64(i)*d)))
	}
	return e
}

func (md model) pickStep(want, span float64) (float64, string) {
	limit := span / minSteps
	reason := "для такой короткой эпидемии"
	if s := 0.1 / md.rate(); s < limit {
		limit = s
		reason = "при таких n и r: число больных меняется слишком быстро"
	}
	if want > limit {
		return limit, fmt.Sprintf("шаг %s сут слишком велик %s, взят шаг %s сут", fg(want), reason, fn(limit, 4))
	}
	if span/want > maxSteps {
		s := span / maxSteps
		return s, fmt.Sprintf("с шагом %s сут счёт шёл бы слишком долго, взят шаг %s сут", fg(want), fn(s, 4))
	}
	return want, ""
}

func niceFloor(x float64) float64 {
	mag := math.Pow(10, math.Floor(math.Log10(x)))
	switch r := x / mag; {
	case r >= 5:
		return 5 * mag
	case r >= 2:
		return 2 * mag
	}
	return mag
}

func section(w io.Writer, title string) {
	fmt.Fprintf(w, "\n%s\n%s\n", title, strings.Repeat("-", len([]rune(title))))
}

func calculate(w io.Writer, n, r, step float64) error {
	md := model{n: n, r: r}
	span := md.end()
	peak := md.peak()
	dt, note := md.pickStep(step, span)
	exact := md.exact(span)
	num := md.advance(n, span, dt)

	section(w, "ИСХОДНЫЕ ДАННЫЕ")
	fmt.Fprintf(w, "  %-34s n = %s чел.\n", "число восприимчивых в сообществе", fg(n))
	fmt.Fprintf(w, "  %-34s r = %s 1/(чел·сут)\n", "частота контактов", fg(r))
	fmt.Fprintf(w, "  %-34s τ = %s сут\n", "шаг метода Рунге-Кутты", fn(dt, 4))
	if note != "" {
		fmt.Fprintf(w, "  (%s)\n", note)
	}

	section(w, "МОДЕЛЬ")
	fmt.Fprintln(w, "  dx/dt = -r·x·(n + 1 - x),   x(0) = n")
	fmt.Fprintln(w, "  x(t) - число незаражённых, n + 1 - x - число заражённых, t - время в сутках")
	fmt.Fprintln(w, "  решение: x(t) = n(n + 1)/(n + e^((n + 1)rt))")

	section(w, "РЕЗУЛЬТАТ")
	if peak > 0 {
		fmt.Fprintf(w, "  Быстрее всего болезнь распространяется в момент tпик = ln n/((n + 1)r) = %s сут:\n", fn(peak, 5))
		fmt.Fprintf(w, "  незаражённых в этот момент (n + 1)/2 = %s чел., заражается %s чел. в сутки.\n", fg((n+1)/2), fn(r*(n+1)*(n+1)/4, 5))
	} else {
		fmt.Fprintf(w, "  Быстрее всего болезнь распространяется в самом начале: заражается %s чел. в сутки.\n", fn(r, 5))
	}
	fmt.Fprintf(w, "  99 %% восприимчивых заразятся к моменту t99 = ln(99n + 100)/((n + 1)r) = %s сут.\n", fn(span, 5))
	fmt.Fprintln(w, "  незаражённых в момент t99, чел.:")
	fmt.Fprintf(w, "    %-22s %s\n", "по формуле", ff(exact, 4))
	fmt.Fprintf(w, "    %-22s %s\n", "методом Рунге-Кутты", ff(num, 4))
	fmt.Fprintf(w, "    %-22s %s\n", "разница", fe(math.Abs(num-exact)))

	section(w, "ПРОВЕРКА ЧИСЛЕННОГО МЕТОДА")
	fmt.Fprintln(w, "  Ошибка - наибольшее отличие от формулы на отрезке от 0 до t99. У метода")
	fmt.Fprintln(w, "  4-го порядка при уменьшении шага вдвое она уменьшается примерно в 16 раз.")
	fmt.Fprintf(w, "  %12s %14s %14s\n", "шаг τ, сут", "ошибка, чел.", "уменьш. в")
	base := niceFloor(math.Min(span/5, 1/md.rate()))
	floor := 1e-13 * (n + 1)
	prev, ratios := 0.0, 0
	for j := 0; j < 6; j++ {
		hs := base / math.Pow(2, float64(j))
		e := md.maxError(span, hs)
		r := "-"
		if j > 0 && prev > floor && e > floor {
			r = ff(prev/e, 1)
			ratios++
		}
		fmt.Fprintf(w, "  %12s %14s %14s\n", fn(hs, 4), fe(e), r)
		prev = e
	}
	if ratios == 0 {
		fmt.Fprintln(w, "  Ошибка на уровне погрешности округления: при таких данных метод")
		fmt.Fprintln(w, "  практически точен уже с крупным шагом.")
	}

	section(w, "ХОД ЭПИДЕМИИ")
	fmt.Fprintf(w, "  %10s %15s %15s %15s %15s\n", "t, сут", "незаражённых", "незаражённых", "заражённых", "заражается")
	fmt.Fprintf(w, "  %10s %15s %15s %15s %15s\n", "", "(формула)", "(РК4)", "", "чел. в сутки")
	td := 3 - int(math.Floor(math.Log10(span)))
	if td < 0 {
		td = 0
	}
	if td > 12 {
		td = 12
	}
	x := n
	for i := 0; i <= rows; i++ {
		t := span * float64(i) / rows
		if i > 0 {
			x = md.advance(x, span/rows, dt)
		}
		e := md.exact(t)
		fmt.Fprintf(w, "  %10s %15s %15s %15s %15s\n", ff(t, td), ff(e, 2), ff(x, 2), ff(n+1-e, 2), ff(r*e*(n+1-e), 2))
	}
	return nil
}

func readLine(r *bufio.Reader) (string, bool) {
	s, err := r.ReadString('\n')
	return strings.TrimRight(s, "\r\n"), err != nil
}

func ask(in *bufio.Reader, w io.Writer, n int, p *param) bool {
	fmt.Fprintf(w, "\n%d. %s\n", n, p.caption())
	fmt.Fprintf(w, "   допустимо от %s до %s; Enter - оставить %s\n", fg(p.min), fg(p.max), fg(p.def))
	for {
		fmt.Fprint(w, "   > ")
		line, eof := readLine(in)
		empty := strings.TrimSpace(line) == ""
		x, err := p.def, error(nil)
		if !empty {
			x, err = parseNumber(line)
		}
		if err == nil {
			err = p.check(x)
		}
		if err == nil {
			p.value = x
			if eof && empty {
				fmt.Fprintln(w)
			}
			return eof
		}
		fmt.Fprintf(w, "   Ошибка: %v.\n   Попробуйте ещё раз.\n", err)
		if eof {
			p.value = p.def
			fmt.Fprintf(w, "   Ввод закончился, беру значение %s.\n", fg(p.def))
			return true
		}
	}
}

func banner(w io.Writer) {
	line := strings.Repeat("=", 66)
	fmt.Fprintln(w, line)
	fmt.Fprintln(w, "  Модель эпидемии: распространение болезни в сообществе")
	fmt.Fprintln(w, line)
	fmt.Fprintln(w, "В сообщество из n восприимчивых людей попадает один заражённый.")
	fmt.Fprintln(w, "Число незаражённых x(t) убывает по закону dx/dt = -r·x·(n + 1 - x).")
	fmt.Fprintln(w, "Программа находит, когда болезнь распространяется быстрее всего и за")
	fmt.Fprintln(w, "какое время заразятся 99 % восприимчивых, по формуле и методом")
	fmt.Fprintln(w, "Рунге-Кутты 4-го порядка.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Как вводить данные:")
	fmt.Fprintln(w, "  * только число, без единиц измерения;")
	fmt.Fprintln(w, "  * дробную часть можно отделять запятой или точкой: 0,0005 или 0.0005;")
	fmt.Fprintln(w, "  * просто Enter - оставить предложенное значение.")
}

func dialog(in *bufio.Reader, w io.Writer, ps []*param) int {
	banner(w)
	for {
		eof := false
		for i, p := range ps {
			if eof {
				p.value = p.def
				continue
			}
			eof = ask(in, w, i+1, p)
		}
		if err := calculate(w, ps[0].value, ps[1].value, ps[2].value); err != nil {
			fmt.Fprintf(w, "\nОшибка: %v\n", err)
		}
		if eof {
			return 0
		}
		for _, p := range ps {
			p.def = p.value
		}
		for {
			fmt.Fprint(w, "\nВведите 1 и нажмите Enter, чтобы посчитать с другими данными,\nили просто нажмите Enter, чтобы выйти: ")
			line, end := readLine(in)
			answer := strings.ToLower(strings.TrimSpace(line))
			if answer == "" || end {
				return 0
			}
			if answer == "1" || answer == "да" || answer == "д" {
				break
			}
			fmt.Fprintln(w, "Не понял ответ.")
		}
	}
}

func normalize(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = a
		if strings.HasPrefix(a, "-") {
			name, val, hasVal := strings.Cut(a, "=")
			out[i] = strings.ToLower(name)
			if hasVal {
				out[i] += "=" + val
			}
		}
	}
	return out
}

func fromFlags(args []string, ps []*param) error {
	for _, a := range args {
		switch strings.ToLower(a) {
		case "/?", "/h", "/help", "?", "help":
			return errHelp
		}
	}
	fset := flag.NewFlagSet("lab", flag.ContinueOnError)
	fset.SetOutput(io.Discard)
	raw := map[string]*string{}
	for _, p := range ps {
		raw[p.key] = fset.String(p.key, "", "")
	}
	if err := fset.Parse(normalize(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return errHelp
		}
		msg := err.Error()
		if name, ok := strings.CutPrefix(msg, "flag provided but not defined: "); ok {
			return fmt.Errorf("неизвестный параметр %s", name)
		}
		if name, ok := strings.CutPrefix(msg, "flag needs an argument: "); ok {
			return fmt.Errorf("после параметра %s нужно написать значение", name)
		}
		return fmt.Errorf("не удалось разобрать параметры (%s)", msg)
	}
	if fset.NArg() > 0 {
		return fmt.Errorf("непонятные слова в командной строке: %s (значение пишется сразу после имени параметра, например -n 1000)", strings.Join(fset.Args(), " "))
	}
	given := map[string]bool{}
	fset.Visit(func(f *flag.Flag) { given[f.Name] = true })
	for _, p := range ps {
		if !given[p.key] {
			p.value = p.def
			if err := p.check(p.def); err != nil {
				return fmt.Errorf("-%s не указан, а значение по умолчанию %s не подходит: %v", p.key, fg(p.def), err)
			}
			continue
		}
		x, err := parseNumber(*raw[p.key])
		if err == nil {
			err = p.check(x)
		}
		if err != nil {
			return fmt.Errorf("-%s: %v", p.key, err)
		}
		p.value = x
	}
	return nil
}

func usage(w io.Writer, ps []*param) {
	name := filepath.Base(os.Args[0])
	fmt.Fprintln(w, "Модель эпидемии: как быстро болезнь распространяется в сообществе.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Запуск:")
	fmt.Fprintf(w, "  %-32s программа сама спросит все данные\n", name)
	fmt.Fprintf(w, "  %-32s расчёт сразу, без вопросов\n", name+" -параметр значение ...")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Параметры (если параметр не указан, берётся значение по умолчанию):")
	for _, p := range ps {
		fmt.Fprintf(w, "  -%-5s %s, от %s до %s, по умолчанию %s\n", p.key, p.caption(), fg(p.min), fg(p.max), fg(p.def))
	}
	fmt.Fprintln(w, "  -help  эта справка")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Примеры:")
	fmt.Fprintf(w, "  %-32s расчёт по значениям по умолчанию\n", name+" -n 1000 -r 0.0005")
	fmt.Fprintf(w, "  %-32s большое сообщество\n", name+" -n 100000 -r 0.000005")
	fmt.Fprintf(w, "  %-32s расчёт с крупным шагом\n", name+" -step 1")
}

func run(args []string, in io.Reader, w, errw io.Writer) int {
	ps := newParams()
	if len(args) == 0 {
		return dialog(bufio.NewReader(in), w, ps)
	}
	err := fromFlags(args, ps)
	if errors.Is(err, errHelp) {
		usage(w, ps)
		return 0
	}
	if err != nil {
		fmt.Fprintf(errw, "Ошибка: %v\n\n", err)
		fmt.Fprintf(errw, "Справка: %s -help\n", filepath.Base(os.Args[0]))
		fmt.Fprintln(errw, "Если запустить программу без параметров, она сама спросит все данные.")
		return 2
	}
	if err := calculate(w, ps[0].value, ps[1].value, ps[2].value); err != nil {
		fmt.Fprintf(errw, "Ошибка: %v\n", err)
		return 1
	}
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
