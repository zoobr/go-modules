package csxutils

import (
	"hash/fnv"
	"reflect"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	// PaySystems is a list of pay systems with first digits
	PaySystems = map[string]string{
		"2":  "Мир",
		"30": "Diners Club",
		"36": "Diners Club",
		"38": "Diners Club",
		"31": "JCB International",
		"35": "JCB International",
		"34": "American Express",
		"37": "American Express",
		"4":  "VISA",
		"50": "Maestro",
		"56": "Maestro",
		"57": "Maestro",
		"58": "Maestro",
		"51": "MasterCard",
		"52": "MasterCard",
		"53": "MasterCard",
		"54": "MasterCard",
		"55": "MasterCard",
		"60": "Discover",
		"62": "China UnionPay",
		"63": "Maestro",
		"67": "Maestro",
		"7":  "УЭК",
	}
	weekDays = map[string]int{
		"Monday":    1,
		"Tuesday":   2,
		"Wednesday": 3,
		"Thursday":  4,
		"Friday":    5,
		"Saturday":  6,
		"Sunday":    7,
	}
)

// Interface for delegating copy process to type
type Interface interface {
	DeepCopy() interface{}
}

// Iface is an alias to Copy; this exists for backwards compatibility reasons.
func Iface(iface interface{}) interface{} {
	return Copy(iface)
}

// Unpstr return string from pointer
func Unpstr(ref *string, def ...string) string {
	var result string
	if ref != nil {
		result = *ref
	}
	if result == "" && len(def) > 0 {
		return def[0]
	}
	return result
}

// UnpstrOr return firstargwith not nil value
func UnpstrOr(refs ...*string) string {
	for i := 0; i < len(refs); i++ {
		ref := refs[i]
		if ref != nil {
			return *ref
		}
	}
	return ""
}

// Unpint return int from pointer
func Unpint(ref *int, def ...int) int {
	var result int
	if ref != nil {
		result = *ref
	}
	if result == 0 && len(def) > 0 {
		return def[0]
	}
	return result
}

// Unpfloat64 return float64 from pointer
func Unpfloat64(ref *float64, def ...float64) float64 {
	var result float64
	if ref != nil {
		result = *ref
	}
	if result == 0 && len(def) > 0 {
		return def[0]
	}
	return result
}

// CopyPtrStr return copy string pointer
func CopyPtrStr(ref *string) *string {
	var copyStr string
	if ref != nil {
		copyStr = *ref
		return &copyStr
	}
	return nil
}

// CopyPtrFloat64 return copy float64 pointer
func CopyPtrFloat64(ref *float64) *float64 {
	var copyFloat64 float64
	if ref != nil {
		copyFloat64 = *ref
		return &copyFloat64
	}
	return nil
}

// CopyPtrInt return copy int pointer
func CopyPtrInt(ref *int) *int {
	var copyInt int
	if ref != nil {
		copyInt = *ref
		return &copyInt
	}
	return nil
}

// CopyPtrTime return copy time pointer
func CopyPtrTime(ref *time.Time) *time.Time {
	var copyTime time.Time
	if ref != nil {
		copyTime = *ref
		return &copyTime
	}
	return nil
}

// Copy creates a deep copy of whatever is passed to it and returns the copy
// in an interface{}.  The returned value will need to be asserted to the
// correct type.
func Copy(src interface{}) interface{} {
	if src == nil {
		return nil
	}

	// Make the interface a reflect.Value
	original := reflect.ValueOf(src)

	// Make a copy of the same type as the original.
	cpy := reflect.New(original.Type()).Elem()

	// Recursively copy the original.
	CopyRecursive(original, cpy)

	// Return the copy as an interface.
	return cpy.Interface()
}

// Assign assign properties
func Assign(from, to interface{}) interface{} {
	if from == nil {
		return nil
	}

	// Make the interface a reflect.Value
	fromValue := reflect.ValueOf(from).Elem()
	toValue := reflect.ValueOf(to).Elem()
	// Recursively copy the original.
	CopyRecursive(fromValue, toValue)

	// Return the copy as an interface.
	return toValue.Interface()
}

// CopyRecursive does the actual copying of the interface. It currently has
// limited support for what it can handle. Add as needed.
func CopyRecursive(original, cpy reflect.Value) {
	// check for implement deepcopy.Interface
	if original.CanInterface() {
		if copier, ok := original.Interface().(Interface); ok {
			cpy.Set(reflect.ValueOf(copier.DeepCopy()))
			return
		}
	}

	// handle according to original's Kind
	switch original.Kind() {
	case reflect.Ptr:
		// Get the actual value being pointed to.
		originalValue := original.Elem()

		// if  it isn't valid, return.
		if !originalValue.IsValid() {
			cpy.Set(reflect.Zero(original.Type()))
			return
		}
		cpy.Set(reflect.New(originalValue.Type()))
		CopyRecursive(originalValue, cpy.Elem())

	case reflect.Interface:
		// If this is a nil, don't do anything
		if original.IsNil() {
			return
		}
		// Get the value for the interface, not the pointer.
		originalValue := original.Elem()

		// Get the value by calling Elem().
		copyValue := reflect.New(originalValue.Type()).Elem()
		CopyRecursive(originalValue, copyValue)
		cpy.Set(copyValue)

	case reflect.Struct:
		t, ok := original.Interface().(time.Time)
		if ok {
			cpy.Set(reflect.ValueOf(t))
			return
		}
		// Go through each field of the struct and copy it.
		for i := 0; i < original.NumField(); i++ {
			// The Type's StructField for a given field is checked to see if StructField.PkgPath
			// is set to determine if the field is exported or not because CanSet() returns false
			// for settable fields.  I'm not sure why.  -mohae
			if original.Type().Field(i).PkgPath != "" {
				continue
			}
			CopyRecursive(original.Field(i), cpy.Field(i))
		}

	case reflect.Slice:
		if original.IsNil() {
			return
		}
		// Make a new slice and copy each element.
		cpy.Set(reflect.MakeSlice(original.Type(), original.Len(), original.Cap()))
		for i := 0; i < original.Len(); i++ {
			CopyRecursive(original.Index(i), cpy.Index(i))
		}

	case reflect.Map:
		if original.IsNil() {
			return
		}
		cpy.Set(reflect.MakeMap(original.Type()))
		for _, key := range original.MapKeys() {
			originalValue := original.MapIndex(key)
			copyValue := reflect.New(originalValue.Type()).Elem()
			CopyRecursive(originalValue, copyValue)
			copyKey := Copy(key.Interface())
			cpy.SetMapIndex(reflect.ValueOf(copyKey), copyValue)
		}

	default:
		cpy.Set(original)
	}
}

// GetPaySystemByBankCard returns pay system by first digits of bank card
func GetPaySystemByBankCard(firstDigits string) string {
	if firstDigits == "" {
		return "Unknown"
	}
	firstDigit := firstDigits[0:1]
	if firstDigit == "2" || firstDigit == "4" || firstDigit == "7" {
		return PaySystems[firstDigit]
	}
	twoDigits := firstDigits[0:2]
	if _, ok := PaySystems[twoDigits]; ok {
		return PaySystems[twoDigits]
	}
	return "Unknown"
}

func LowerFirst(s string) string {
	if s == "" {
		return ""
	}
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToLower(r)) + s[n:]
}

// GetFNV1aHash32 returns FNV-1a 32 bit hash
func GetFNV1aHash32(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

func IsNilInterface(i interface{}) bool {
	if i == nil {
		return true
	}
	switch reflect.TypeOf(i).Kind() {
	case reflect.Ptr, reflect.Map, reflect.Array, reflect.Chan, reflect.Slice:
		return reflect.ValueOf(i).IsNil()
	}
	return false
}

// Weekday Returns the number of the day of the week starting from Monday
func Weekday(date *time.Time) int {
	if date == nil {
		return -1
	}
	return weekDays[date.UTC().Weekday().String()]
}
