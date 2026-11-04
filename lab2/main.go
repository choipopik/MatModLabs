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
	hour     = 60.0
	minSteps = 20
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
	above *param
	value float64
}

func newParams() []*param {
	a := &param{key: "a", title: "Начальная температура металла и печи", unit: "°C", def: 20, min: -273.15, max: 3000,
		low:  "температура не может быть ниже абсолютного нуля",
		high: "программа рассчитана на печи с температурой не выше 3000 °C"}
	b := &param{key: "b", title: "Температура печи через час", unit: "°C", hint: "должна быть больше начальной", def: 1000, min: -273.15, max: 3000,
		low:  "температура не может быть ниже абсолютного нуля",
		high: "программа рассчитана на печи с температурой не выше 3000 °C", above: a}
	k := &param{key: "k", title: "Коэффициент k (скорость нагрева kT при разности температур T)", unit: "1/мин", def: 0.05, min: 1e-6, max: 100,
		low:  "коэффициент должен быть больше нуля, иначе металл не нагревается",
		high: "при таком k металл почти мгновенно принимает температуру печи"}
	step := &param{key: "step", title: "Шаг численного метода Рунге-Кутты", unit: "мин", def: 1, min: 1e-4, max: 10,
		low:  "с таким маленьким шагом счёт шёл бы слишком долго",
		high: "с таким большим шагом результат получается слишком грубым"}
	return []*param{a, b, k, step}
}

func (p *param) check(x float64) error {
	if x < p.min {
		return fmt.Errorf("значение должно быть от %s до %s %s: %s", fg(p.min), fg(p.max), p.unit, p.low)
	}
	if x > p.max {
		return fmt.Errorf("значение должно быть от %s до %s %s: %s", fg(p.min), fg(p.max), p.unit, p.high)
	}
	if p.above != nil && x <= p.above.value {
		return fmt.Errorf("значение должно быть больше начальной температуры %s %s, потому что печь нагревается", fg(p.above.value), p.unit)
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
		return 0, fmt.Errorf("«%s» - это не число (пример правильного ввода: 0,05 или 0.05)", short(s))
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
	a, b, k float64
}

func (md model) oven(t float64) float64 {
	return md.a + (md.b-md.a)*t/hour
}

func (md model) exact(t float64) float64 {
	c := (md.b - md.a) / hour
	return md.oven(t) + c/md.k*math.Expm1(-md.k*t)
}

func (md model) f(t, x float64) float64 {
	return md.k * (md.oven(t) - x)
}

func (md model) step(t, x, dt float64) float64 {
	k1 := md.f(t, x)
	k2 := md.f(t+dt/2, x+dt/2*k1)
	k3 := md.f(t+dt/2, x+dt/2*k2)
	k4 := md.f(t+dt, x+dt*k3)
	return x + dt/6*(k1+2*k2+2*k3+k4)
}

func (md model) advance(t, x, dur, dt float64) float64 {
	n := int(math.Ceil(dur/dt - 1e-9))
	d := dur / float64(n)
	for i := 0; i < n; i++ {
		x = md.step(t+float64(i)*d, x, d)
	}
	return x
}

func (md model) maxError(dt float64) float64 {
	n := int(math.Ceil(hour/dt - 1e-9))
	d := hour / float64(n)
	x, e := md.a, 0.0
	for i := 1; i <= n; i++ {
		x = md.step(float64(i-1)*d, x, d)
		e = math.Max(e, math.Abs(x-md.exact(float64(i)*d)))
	}
	return e
}

func (md model) pickStep(want float64) (float64, string) {
	limit := hour / minSteps
	reason := "для расчёта на один час"
	if s := 0.5 / md.k; s < limit {
		limit = s
		reason = "при таком большом k"
	}
	note := ""
	if want > limit {
		note = fmt.Sprintf("шаг %s мин слишком велик %s, ", fg(want), reason)
		want = limit
	}
	n := math.Ceil(hour/want - 1e-9)
	dt := hour / n
	if note != "" {
		note += fmt.Sprintf("взят шаг %s мин", fn(dt, 4))
	} else if math.Abs(dt-want) > 1e-12*want {
		note = fmt.Sprintf("шаг взят %s мин, чтобы час делился на целое число шагов", fn(dt, 4))
	}
	return dt, note
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

func calculate(w io.Writer, a, b, k, step float64) error {
	if b <= a {
		return fmt.Errorf("температура печи через час (%s °C) должна быть больше начальной (%s °C)", fg(b), fg(a))
	}
	md := model{a: a, b: b, k: k}
	dt, note := md.pickStep(step)
	exact := md.exact(hour)
	num := md.advance(0, a, hour, dt)

	section(w, "ИСХОДНЫЕ ДАННЫЕ")
	fmt.Fprintf(w, "  %-38s a = %s °C\n", "начальная температура металла и печи", fg(a))
	fmt.Fprintf(w, "  %-38s b = %s °C\n", "температура печи через час", fg(b))
	fmt.Fprintf(w, "  %-38s k = %s 1/мин\n", "коэффициент нагрева", fg(k))
	fmt.Fprintf(w, "  %-38s τ = %s мин\n", "шаг метода Рунге-Кутты", fn(dt, 4))
	if note != "" {
		fmt.Fprintf(w, "  (%s)\n", note)
	}

	section(w, "МОДЕЛЬ")
	fmt.Fprintln(w, "  dx/dt = k·(θ(t) - x),   θ(t) = a + (b - a)·t/60,   x(0) = a")
	fmt.Fprintln(w, "  x(t) - температура металла, θ(t) - температура печи, t - время в минутах")
	fmt.Fprintln(w, "  решение: x(60) = b - (b - a)·(1 - e^(-60k))/(60k)")

	section(w, "РЕЗУЛЬТАТ")
	fmt.Fprintln(w, "  температура металла через час, °C:")
	fmt.Fprintf(w, "    %-22s %s\n", "по формуле", ff(exact, 4))
	fmt.Fprintf(w, "    %-22s %s\n", "методом Рунге-Кутты", ff(num, 4))
	fmt.Fprintf(w, "    %-22s %s\n", "разница", fe(math.Abs(num-exact)))
	fmt.Fprintf(w, "  Ответ: через час металл нагреется до %s °C, печь - до %s °C,\n", ff(exact, 1), fg(b))
	fmt.Fprintf(w, "  т.е. металл холоднее печи на %s °C.\n", ff(b-exact, 1))

	section(w, "ПРОВЕРКА ЧИСЛЕННОГО МЕТОДА")
	fmt.Fprintln(w, "  Ошибка - наибольшее отличие от формулы на всём часе нагрева. У метода")
	fmt.Fprintln(w, "  4-го порядка при уменьшении шага вдвое она уменьшается примерно в 16 раз.")
	fmt.Fprintf(w, "  %12s %14s %14s\n", "шаг τ, мин", "ошибка, °C", "уменьш. в")
	base := niceFloor(math.Min(hour/5, 1/k))
	floor := 1e-13 * math.Max(math.Abs(a), math.Abs(b))
	prev, ratios := 0.0, 0
	for j := 0; j < 6; j++ {
		hs := base / math.Pow(2, float64(j))
		e := md.maxError(hs)
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

	section(w, "НАГРЕВ ВО ВРЕМЕНИ")
	fmt.Fprintf(w, "  %8s %12s %14s %14s %14s\n", "t, мин", "печь, °C", "металл, °C", "металл, °C", "разность, °C")
	fmt.Fprintf(w, "  %8s %12s %14s %14s %14s\n", "", "", "(формула)", "(РК4)", "")
	x := a
	for i := 0; i <= rows; i++ {
		t := hour * float64(i) / rows
		if i > 0 {
			x = md.advance(hour*float64(i-1)/rows, x, hour/rows, dt)
		}
		fmt.Fprintf(w, "  %8s %12s %14s %14s %14s\n", fg(t), ff(md.oven(t), 2), ff(md.exact(t), 2), ff(x, 2), ff(md.oven(t)-md.exact(t), 2))
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
	fmt.Fprintln(w, "  Нагрев куска металла в печи: температура металла через час")
	fmt.Fprintln(w, line)
	fmt.Fprintln(w, "Температура печи за час равномерно растёт от a до b. Металл нагревается")
	fmt.Fprintln(w, "со скоростью kT градусов в минуту, где T - разность температур печи и")
	fmt.Fprintln(w, "металла. Программа считает температуру металла через час двумя")
	fmt.Fprintln(w, "способами: по точной формуле и методом Рунге-Кутты 4-го порядка.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Как вводить данные:")
	fmt.Fprintln(w, "  * только число, без единиц измерения;")
	fmt.Fprintln(w, "  * дробную часть можно отделять запятой или точкой: 0,05 или 0.05;")
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
		if err := calculate(w, ps[0].value, ps[1].value, ps[2].value, ps[3].value); err != nil {
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
		return fmt.Errorf("непонятные слова в командной строке: %s (значение пишется сразу после имени параметра, например -k 0.05)", strings.Join(fset.Args(), " "))
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
	fmt.Fprintln(w, "Расчёт температуры куска металла через час нагрева в печи.")
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
	fmt.Fprintf(w, "  %-32s расчёт по значениям по умолчанию\n", name+" -a 20 -b 1000 -k 0.05")
	fmt.Fprintf(w, "  %-32s медленный нагрев\n", name+" -k 0.01")
	fmt.Fprintf(w, "  %-32s расчёт с крупным шагом\n", name+" -step 2")
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
	if err := calculate(w, ps[0].value, ps[1].value, ps[2].value, ps[3].value); err != nil {
		fmt.Fprintf(errw, "Ошибка: %v\n", err)
		return 1
	}
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
