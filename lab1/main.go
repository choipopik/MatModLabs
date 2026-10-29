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
	gramForce = 1e-3
	maxSteps  = 1000000
	minAscent = 25
	rows      = 10
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
		{key: "mass", title: "Масса (вес) мяча", unit: "кг", def: 0.4, min: 0.001, max: 1000,
			low:  "масса должна быть больше нуля (не меньше 1 г)",
			high: "программа рассчитана на тела массой не больше 1 т"},
		{key: "v0", title: "Начальная скорость броска вверх", unit: "м/с", def: 20, min: 0.1, max: 340,
			low:  "мяч должен быть брошен вверх, то есть скорость должна быть больше нуля",
			high: "при скорости больше скорости звука (около 340 м/с) закон сопротивления ~v² перестаёт работать"},
		{key: "drag", title: "Сила сопротивления воздуха при скорости 1 м/с", unit: "гс", hint: "граммы силы, 0 - без сопротивления", def: 0.48, min: 0, max: 100000,
			low:  "сила сопротивления не может быть отрицательной (0 - без сопротивления)",
			high: "при таком сопротивлении мяч почти не сдвинется с места"},
		{key: "g", title: "Ускорение свободного падения", unit: "м/с²", def: 9.81, min: 0.1, max: 100,
			low:  "сила тяжести должна тянуть мяч вниз, иначе он никогда не остановится",
			high: "это намного больше, чем на любой планете Солнечной системы"},
		{key: "step", title: "Шаг численного метода Рунге-Кутты", unit: "с", def: 0.01, min: 1e-6, max: 1,
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
	if p.key == "drag" && x > 0 && x < 1e-6 {
		return errors.New("такое сопротивление слишком мало, чтобы его заметить; если сопротивление не нужно, введите 0")
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
	s = strings.TrimSpace(strings.TrimPrefix(s, "\xef\xbb\xbf"))
	if s == "" {
		return 0, errEmpty
	}
	if !numRe.MatchString(s) {
		if strings.Count(s, ".")+strings.Count(s, ",") > 1 {
			return 0, fmt.Errorf("«%s» - в числе может быть только одна точка или запятая", short(s))
		}
		if lead := leadRe.FindString(s); lead != "" {
			rest := []rune(strings.TrimSpace(s[len(lead):]))
			if len(rest) > 0 && unicode.IsDigit(rest[0]) {
				return 0, fmt.Errorf("«%s» - уберите пробелы внутри числа", short(s))
			}
			if len(rest) > 0 && (unicode.IsLetter(rest[0]) || rest[0] == '/' || rest[0] == '°') {
				return 0, fmt.Errorf("«%s» - единицы измерения писать не нужно, введите только число", short(s))
			}
		}
		return 0, fmt.Errorf("«%s» - это не число (пример правильного ввода: 0,4 или 0.4)", short(s))
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

func decFor(x float64, sig int) int {
	if x == 0 || math.IsInf(x, 0) || math.IsNaN(x) {
		return sig - 1
	}
	d := sig - 1 - int(math.Floor(math.Log10(math.Abs(x))))
	if d < 0 {
		d = 0
	}
	if d > 12 {
		d = 12
	}
	return d
}

func fs(x float64, sig int) string {
	return ff(x, decFor(x, sig))
}

func fn(x float64, sig int) string {
	s := fs(x, sig)
	if strings.Contains(s, ",") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ",")
	}
	return s
}

type model struct {
	g, c, vt, v0, phi float64
}

func newModel(mass, v0, drag, g float64) model {
	k := drag * gramForce * g
	md := model{g: g, c: k / mass, v0: v0, vt: math.Inf(1)}
	if md.c > 0 {
		md.vt = math.Sqrt(g / md.c)
		md.phi = math.Atan(v0 / md.vt)
	}
	return md
}

func (md model) exact() (float64, float64) {
	if md.c == 0 {
		return md.v0 / md.g, md.v0 * md.v0 / (2 * md.g)
	}
	return md.vt / md.g * md.phi, md.vt * md.vt / (2 * md.g) * math.Log1p(md.v0*md.v0/(md.vt*md.vt))
}

func (md model) at(t float64) (float64, float64) {
	if md.c == 0 {
		return md.v0 - md.g*t, md.v0*t - md.g*t*t/2
	}
	b := md.g * t / (2 * md.vt)
	return md.vt * math.Tan(md.phi-2*b), md.vt * md.vt / md.g * math.Log1p(2*math.Sin(md.phi-b)*math.Sin(b)/math.Cos(md.phi))
}

func (md model) f(v float64) float64 {
	return -md.g - md.c*v*math.Abs(v)
}

func (md model) step(h, v, dt float64) (float64, float64) {
	k1 := md.f(v)
	v2 := v + dt/2*k1
	k2 := md.f(v2)
	v3 := v + dt/2*k2
	k3 := md.f(v3)
	v4 := v + dt*k3
	k4 := md.f(v4)
	return h + dt/6*(v+2*v2+2*v3+v4), v + dt/6*(k1+2*k2+2*k3+k4)
}

func (md model) ascent(dt float64) (float64, float64, error) {
	h, v := 0.0, md.v0
	limit := 2*int(math.Ceil(md.v0/(md.g*dt))) + 10
	for i := 0; i < limit; i++ {
		hn, vn := md.step(h, v, dt)
		if math.IsNaN(vn) || math.IsInf(vn, 0) || math.IsNaN(hn) || math.IsInf(hn, 0) {
			return 0, 0, errors.New("численный метод разошёлся, попробуйте уменьшить шаг")
		}
		if vn > 0 {
			h, v = hn, vn
			continue
		}
		lo, hi := 0.0, dt
		for j := 0; j < 200; j++ {
			mid := (lo + hi) / 2
			if mid <= lo || mid >= hi {
				break
			}
			if _, vm := md.step(h, v, mid); vm > 0 {
				lo = mid
			} else {
				hi = mid
			}
		}
		tau := (lo + hi) / 2
		hT, _ := md.step(h, v, tau)
		return float64(i)*dt + tau, hT, nil
	}
	return 0, 0, errors.New("численный метод не нашёл момент остановки мяча")
}

func (md model) bound() float64 {
	b := md.v0 / md.g
	if md.c > 0 {
		b = math.Min(b, math.Pi*md.vt/(2*md.g))
	}
	return b
}

func (md model) pickStep(want float64) (float64, string) {
	b := md.bound()
	limit := b / minAscent
	reason := "для такого короткого подъёма"
	if md.c > 0 {
		if s := 0.25 / (md.c * md.v0); s < limit {
			limit = s
			reason = "при таком сильном сопротивлении: скорость в начале меняется слишком быстро"
		}
	}
	if want > limit {
		return limit, fmt.Sprintf("шаг %s с слишком велик %s, взят шаг %s с", fg(want), reason, fn(limit, 4))
	}
	if b/want > maxSteps {
		s := math.Min(b/maxSteps, limit)
		return s, fmt.Sprintf("с шагом %s с счёт шёл бы слишком долго, взят шаг %s с", fg(want), fn(s, 4))
	}
	return want, ""
}

func (md model) advance(h, v, dur, dt float64) (float64, float64) {
	n := int(math.Ceil(dur/dt - 1e-9))
	if n < 1 {
		n = 1
	}
	d := dur / float64(n)
	for i := 0; i < n; i++ {
		h, v = md.step(h, v, d)
	}
	return h, v
}

type point struct {
	t, v, h, vn, hn, vf, hf float64
}

func trajectory(md, free model, T, dt float64) []point {
	pts := make([]point, rows+1)
	h, v := 0.0, md.v0
	for i := 0; i <= rows; i++ {
		t := T * float64(i) / rows
		if i > 0 {
			h, v = md.advance(h, v, T/rows, dt)
		}
		va, ha := md.at(t)
		vf, hf := free.at(t)
		pts[i] = point{t, va, ha, v, h, vf, hf}
	}
	return pts
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

func calculate(w io.Writer, mass, v0, drag, g, step float64) error {
	md := newModel(mass, v0, drag, g)
	free := newModel(mass, v0, 0, g)
	withDrag := md.c > 0
	dt, note := md.pickStep(step)
	dtFree, noteFree := free.pickStep(step)
	if !withDrag {
		dt, note = dtFree, noteFree
	}

	T, H := md.exact()
	T0, H0 := free.exact()
	Tn, Hn, err := md.ascent(dt)
	if err != nil {
		return err
	}
	T0n, H0n, err := free.ascent(dtFree)
	if err != nil {
		return err
	}
	k := drag * gramForce * g

	section(w, "ИСХОДНЫЕ ДАННЫЕ")
	fmt.Fprintf(w, "  %-36s m  = %s кг\n", "масса мяча", fg(mass))
	fmt.Fprintf(w, "  %-36s v0 = %s м/с\n", "начальная скорость", fg(v0))
	fmt.Fprintf(w, "  %-36s F1 = %s гс = %s Н\n", "сопротивление при скорости 1 м/с", fg(drag), fn(k, 5))
	fmt.Fprintf(w, "  %-36s g  = %s м/с²\n", "ускорение свободного падения", fg(g))
	fmt.Fprintf(w, "  %-36s τ  = %s с\n", "шаг метода Рунге-Кутты", fn(dt, 4))
	if note != "" {
		fmt.Fprintf(w, "  (%s)\n", note)
	}
	if withDrag && dtFree != dt {
		fmt.Fprintf(w, "  (для расчёта без сопротивления взят шаг %s с)\n", fn(dtFree, 4))
	}

	section(w, "МОДЕЛЬ")
	fmt.Fprintln(w, "  m·dv/dt = -m·g - k·v²,   dh/dt = v,   v(0) = v0,   h(0) = 0")
	fmt.Fprintln(w, "  подъём заканчивается в момент T, когда v(T) = 0; наибольшая высота H = h(T)")
	if withDrag {
		fmt.Fprintf(w, "  коэффициент сопротивления  k = F1/(1 м/с)² = %s Н·с²/м²\n", fn(k, 5))
		fmt.Fprintf(w, "  скорость, при которой сопротивление равно весу  vуст = sqrt(m·g/k) = %s м/с\n", fn(md.vt, 6))
		if r := v0 * v0 / (md.vt * md.vt); r < 1 {
			fmt.Fprintf(w, "  в момент броска сопротивление составляет %s %% веса мяча\n", fn(100*r, 3))
		} else {
			fmt.Fprintf(w, "  в момент броска сопротивление больше веса мяча в %s раза\n", fn(r, 3))
		}
	} else {
		fmt.Fprintln(w, "  сопротивление равно нулю, поэтому оба случая (с сопротивлением и без) совпадают")
	}

	section(w, "РЕЗУЛЬТАТЫ")
	fmt.Fprintf(w, "  %-30s %20s %26s\n", "", "время подъёма T, с", "наибольшая высота H, м")
	block := func(title string, te, he, tn, hn float64) {
		fmt.Fprintf(w, "  %s\n", title)
		fmt.Fprintf(w, "    %-28s %20s %26s\n", "по точной формуле", fs(te, 7), fs(he, 7))
		fmt.Fprintf(w, "    %-28s %20s %26s\n", "методом Рунге-Кутты", fs(tn, 7), fs(hn, 7))
		fmt.Fprintf(w, "    %-28s %20s %26s\n", "разница", fe(math.Abs(tn-te)), fe(math.Abs(hn-he)))
	}
	if withDrag {
		block("с сопротивлением воздуха", T, H, Tn, Hn)
	}
	block("без сопротивления воздуха", T0, H0, T0n, H0n)

	section(w, "ОТВЕТ")
	if withDrag {
		fmt.Fprintf(w, "  С сопротивлением воздуха мяч поднимается %s с на высоту %s м.\n", fs(T, 4), fs(H, 4))
		fmt.Fprintf(w, "  Если сопротивлением пренебречь, время подъёма станет %s с (больше на %s %%),\n", fs(T0, 4), ff(100*(T0-T)/T, 1))
		fmt.Fprintf(w, "  а наибольшая высота - %s м (больше на %s %%).\n", fs(H0, 4), ff(100*(H0-H)/H, 1))
		fmt.Fprintf(w, "  Иначе говоря, сопротивление воздуха уменьшает время подъёма на %s %% и высоту на %s %%.\n", ff(100*(T0-T)/T0, 1), ff(100*(H0-H)/H0, 1))
	} else {
		fmt.Fprintf(w, "  Без сопротивления мяч поднимается %s с на высоту %s м.\n", fs(T0, 4), fs(H0, 4))
	}

	section(w, "ПРОВЕРКА ЧИСЛЕННОГО МЕТОДА")
	if withDrag {
		fmt.Fprintln(w, "  У метода Рунге-Кутты 4-го порядка при уменьшении шага вдвое ошибка")
		fmt.Fprintln(w, "  должна уменьшаться примерно в 2^4 = 16 раз.")
		fmt.Fprintf(w, "  %12s %14s %14s %14s %14s\n", "шаг τ, с", "ошибка T, с", "уменьш. в", "ошибка H, м", "уменьш. в")
		base := niceFloor(math.Min(md.bound()/5, 1/(md.c*v0)))
		var pt, ph float64
		for j := 0; j < 6; j++ {
			hs := base / math.Pow(2, float64(j))
			tj, hj, err := md.ascent(hs)
			if err != nil {
				return err
			}
			et, eh := math.Abs(tj-T), math.Abs(hj-H)
			rt, rh := "-", "-"
			if j > 0 && et > 1e-13*T && pt > 0 {
				rt = ff(pt/et, 1)
			}
			if j > 0 && eh > 1e-13*H && ph > 0 {
				rh = ff(ph/eh, 1)
			}
			fmt.Fprintf(w, "  %12s %14s %14s %14s %14s\n", fn(hs, 4), fe(et), rt, fe(eh), rh)
			pt, ph = et, eh
		}
	} else {
		fmt.Fprintln(w, "  Без сопротивления скорость убывает линейно, и метод Рунге-Кутты даёт точный")
		fmt.Fprintln(w, "  ответ при любом шаге: разница с формулой возникает только из-за округлений.")
	}

	pts := trajectory(md, free, T, dt)
	section(w, "ДВИЖЕНИЕ МЯЧА ВО ВРЕМЕНИ")
	if withDrag {
		fmt.Fprintln(w, "  формула и РК4 - с сопротивлением, без сопр. - по формуле без сопротивления")
		fmt.Fprintf(w, "  %9s %11s %11s %11s %11s %13s %13s\n", "t, с", "v формула", "v РК4", "h формула", "h РК4", "v без сопр.", "h без сопр.")
	} else {
		fmt.Fprintf(w, "  %9s %11s %11s %11s %11s\n", "t, с", "v формула", "v РК4", "h формула", "h РК4")
	}
	dv, dh, dhf, dtt := decFor(v0, 5), decFor(H, 5), decFor(H0, 5), decFor(T, 4)
	if !withDrag {
		dh = dhf
	}
	for _, p := range pts {
		fmt.Fprintf(w, "  %9s %11s %11s %11s %11s", ff(p.t, dtt), ff(p.v, dv), ff(p.vn, dv), ff(p.h, dh), ff(p.hn, dh))
		if withDrag {
			fmt.Fprintf(w, " %13s %13s", ff(p.vf, dv), ff(p.hf, dhf))
		}
		fmt.Fprintln(w)
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
		if strings.TrimSpace(line) == "" {
			p.value = p.def
			if eof {
				fmt.Fprintln(w)
			}
			return eof
		}
		x, err := parseNumber(line)
		if err == nil {
			err = p.check(x)
		}
		if err == nil {
			p.value = x
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
	fmt.Fprintln(w, "  Мяч, брошенный вертикально вверх: время подъёма и наибольшая высота")
	fmt.Fprintln(w, line)
	fmt.Fprintln(w, "Программа считает, сколько времени мяч летит вверх и на какую высоту")
	fmt.Fprintln(w, "он поднимается - с сопротивлением воздуха и без него. Расчёт ведётся")
	fmt.Fprintln(w, "двумя способами: по точным формулам и численно, методом Рунге-Кутты")
	fmt.Fprintln(w, "4-го порядка.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Как вводить данные:")
	fmt.Fprintln(w, "  * только число, без единиц измерения;")
	fmt.Fprintln(w, "  * дробную часть можно отделять запятой или точкой: 0,4 или 0.4;")
	fmt.Fprintln(w, "  * просто Enter - оставить предложенное значение (сначала это данные из задачи).")
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
		if err := calculate(w, ps[0].value, ps[1].value, ps[2].value, ps[3].value, ps[4].value); err != nil {
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
		return fmt.Errorf("непонятные слова в командной строке: %s (значение пишется сразу после имени параметра, например -v0 20)", strings.Join(fset.Args(), " "))
	}
	given := map[string]bool{}
	fset.Visit(func(f *flag.Flag) { given[f.Name] = true })
	for _, p := range ps {
		p.value = p.def
		if !given[p.key] {
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
	fmt.Fprintln(w, "Расчёт подъёма мяча, брошенного вертикально вверх, с сопротивлением воздуха и без него.")
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
	fmt.Fprintf(w, "  %-32s расчёт по данным задачи\n", name+" -mass 0.4 -v0 20 -drag 0.48")
	fmt.Fprintf(w, "  %-32s расчёт без сопротивления воздуха\n", name+" -drag 0")
	fmt.Fprintf(w, "  %-32s расчёт с крупным шагом\n", name+" -step 0.1")
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
	if err := calculate(w, ps[0].value, ps[1].value, ps[2].value, ps[3].value, ps[4].value); err != nil {
		fmt.Fprintf(errw, "Ошибка: %v\n", err)
		return 1
	}
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
