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
	iterations = 50
	rows       = 10
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
	value float64
}

func newParams() []*param {
	return []*param{
		{key: "x2", title: "Координата x2 точки M2 (по горизонтали)", unit: "м", def: 10, min: 0.001, max: 1000,
			low:  "точка M2 должна быть правее M1, а при x2 = 0 шарик просто падает вертикально",
			high: "программа рассчитана на расстояния не больше 1000 м"},
		{key: "y2", title: "Координата y2 точки M2 (глубина, ось Y направлена вниз)", unit: "м", def: 3, min: 0.001, max: 1000,
			low:  "точка M2 должна быть ниже M1, иначе шарик без начальной скорости в неё не скатится",
			high: "программа рассчитана на расстояния не больше 1000 м"},
		{key: "g", title: "Ускорение свободного падения", unit: "м/с²", def: 9.81, min: 0.1, max: 100,
			low:  "сила тяжести должна тянуть шарик вниз, иначе он не сдвинется с места",
			high: "это намного больше, чем на любой планете Солнечной системы"},
	}
}

func (p *param) check(x float64) error {
	if x < p.min {
		return fmt.Errorf("значение должно быть от %s до %s %s: %s", fg(p.min), fg(p.max), p.unit, p.low)
	}
	if x > p.max {
		return fmt.Errorf("значение должно быть от %s до %s %s: %s", fg(p.min), fg(p.max), p.unit, p.high)
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
		return 0, fmt.Errorf("«%s» - это не число (пример правильного ввода: 9,81 или 9.81)", short(s))
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

func sub(t float64) float64 {
	if t < 0.1 {
		t2 := t * t
		return t * t2 / 6 * (1 - t2/20*(1-t2/42*(1-t2/72)))
	}
	return t - math.Sin(t)
}

func cosm(t float64) float64 {
	s := math.Sin(t / 2)
	return 2 * s * s
}

func ratio(t float64) float64 {
	return sub(t) / cosm(t)
}

type iter struct {
	n                   int
	theta, width, resid float64
}

func solve(q float64) (float64, []iter) {
	lo, hi := 0.0, 2*math.Pi
	var log []iter
	for i := 1; i <= iterations; i++ {
		mid := (lo + hi) / 2
		if ratio(mid) < q {
			lo = mid
		} else {
			hi = mid
		}
		t := (lo + hi) / 2
		log = append(log, iter{i, t, hi - lo, math.Abs(ratio(t)-q) / q})
	}
	return (lo + hi) / 2, log
}

func dec(scale float64, sig int) int {
	d := sig - 1 - int(math.Floor(math.Log10(scale)))
	if d < 0 {
		return 0
	}
	if d > 12 {
		return 12
	}
	return d
}

func section(w io.Writer, title string) {
	fmt.Fprintf(w, "\n%s\n%s\n", title, strings.Repeat("-", len([]rune(title))))
}

func calculate(w io.Writer, x2, y2, g float64) error {
	th, log := solve(x2 / y2)
	R := y2 / cosm(th)
	T := th * math.Sqrt(R/g)
	L := math.Hypot(x2, y2)
	line := L * math.Sqrt(2/(g*y2))

	section(w, "ИСХОДНЫЕ ДАННЫЕ")
	fmt.Fprintf(w, "  %-30s M1(0; 0)\n", "начальная точка")
	fmt.Fprintf(w, "  %-30s M2(%s; %s), м\n", "конечная точка", fg(x2), fg(y2))
	fmt.Fprintf(w, "  %-30s g = %s м/с²\n", "ускорение свободного падения", fg(g))
	fmt.Fprintln(w, "  ось X направлена вправо, ось Y - вниз")

	section(w, "МОДЕЛЬ")
	fmt.Fprintln(w, "  время спуска T[y] = интеграл от 0 до x2 от sqrt(1 + y'²)/sqrt(2gy) dx -> min")
	fmt.Fprintln(w, "  y(0) = 0, y(x2) = y2")
	fmt.Fprintln(w, "  экстремаль - циклоида: x = R(θ - sin θ), y = R(1 - cos θ), 0 <= θ <= θ2")
	fmt.Fprintln(w, "  θ2 находится из уравнения (θ - sin θ)/(1 - cos θ) = x2/y2, затем R = y2/(1 - cos θ2)")

	section(w, "РЕШЕНИЕ УРАВНЕНИЯ ДЛЯ θ2")
	fmt.Fprintln(w, "  Метод деления отрезка пополам на (0; 2π): левая часть уравнения возрастает,")
	fmt.Fprintln(w, "  поэтому корень единственный, а после каждого шага отрезок с корнем вдвое короче.")
	fmt.Fprintf(w, "  %6s %18s %16s %16s\n", "шаг", "θ, рад", "длина отрезка", "невязка")
	for _, it := range log {
		if it.n%5 == 0 {
			fmt.Fprintf(w, "  %6d %18s %16s %16s\n", it.n, ff(it.theta, 12), fe(it.width), fe(it.resid))
		}
	}

	section(w, "РЕЗУЛЬТАТ")
	fmt.Fprintf(w, "  θ2 = %s рад (%s°), R = %s м\n", ff(th, 6), ff(th*180/math.Pi, 2), fn(R, 7))
	fmt.Fprintf(w, "  проверка: кривая кончается в точке (%s; %s), м\n", fn(R*sub(th), 9), fn(R*cosm(th), 9))
	fmt.Fprintf(w, "  время спуска по брахистохроне   T = θ2·sqrt(R/g) = %s с\n", fn(T, 5))
	fmt.Fprintf(w, "  время спуска по прямой M1M2         %s с (дольше на %s %%)\n", fn(line, 5), ff(100*(line-T)/T, 1))
	if th > math.Pi {
		fmt.Fprintf(w, "  шарик сначала опускается ниже M2, до точки (%s; %s) м, а затем поднимается к M2\n", fn(math.Pi*R, 5), fn(2*R, 5))
	}

	section(w, "ТОЧКИ БРАХИСТОХРОНЫ")
	fmt.Fprintf(w, "  %10s %12s %12s %14s %12s\n", "θ, рад", "x, м", "y, м", "скорость, м/с", "время, с")
	dx := dec(math.Max(x2, 2*R), 5)
	dv := dec(math.Sqrt(2*g*2*R), 5)
	dt := dec(T, 5)
	for i := 0; i <= rows; i++ {
		t := th * float64(i) / rows
		y := R * cosm(t)
		fmt.Fprintf(w, "  %10s %12s %12s %14s %12s\n", ff(t, 4), ff(R*sub(t), dx), ff(y, dx), ff(math.Sqrt(2*g*y), dv), ff(t*math.Sqrt(R/g), dt))
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
	fmt.Fprintln(w, "  Брахистохрона: кривая наискорейшего спуска")
	fmt.Fprintln(w, line)
	fmt.Fprintln(w, "Шарик без начальной скорости скатывается из точки M1(0; 0) в точку")
	fmt.Fprintln(w, "M2(x2; y2), ось Y направлена вниз. Программа находит кривую, по которой")
	fmt.Fprintln(w, "спуск занимает меньше всего времени (дугу циклоиды), и время спуска.")
	fmt.Fprintln(w, "Параметр циклоиды находится методом деления отрезка пополам.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Как вводить данные:")
	fmt.Fprintln(w, "  * только число, без единиц измерения;")
	fmt.Fprintln(w, "  * дробную часть можно отделять запятой или точкой: 9,81 или 9.81;")
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
		return fmt.Errorf("непонятные слова в командной строке: %s (значение пишется сразу после имени параметра, например -x2 10)", strings.Join(fset.Args(), " "))
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
	fmt.Fprintln(w, "Задача о брахистохроне: кривая наискорейшего спуска из M1(0; 0) в M2(x2; y2).")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Запуск:")
	fmt.Fprintf(w, "  %-32s программа сама спросит все данные\n", name)
	fmt.Fprintf(w, "  %-32s расчёт сразу, без вопросов\n", name+" -параметр значение ...")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Параметры (если параметр не указан, берётся значение из задачи):")
	for _, p := range ps {
		fmt.Fprintf(w, "  -%-5s %s, от %s до %s, по умолчанию %s\n", p.key, p.caption(), fg(p.min), fg(p.max), fg(p.def))
	}
	fmt.Fprintln(w, "  -help  эта справка")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Примеры:")
	fmt.Fprintf(w, "  %-32s расчёт по данным задачи\n", name+" -x2 10 -y2 3")
	fmt.Fprintf(w, "  %-32s точка M2 почти под M1\n", name+" -x2 1 -y2 10")
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
