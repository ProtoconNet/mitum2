package util

import (
	"reflect"
)

func InStringSlice(n string, s []string) bool {
	for _, i := range s {
		if n == i {
			return true
		}
	}

	return false
}

func CheckSliceDuplicated(s interface{}, key func(interface{}) string) bool {
	if s == nil {
		return false
	}

	sl := makeInterfaceSlice(s)
	if sl == nil {
		return false
	}

	m := map[string]struct{}{}
	for i := range sl {
		var k string
		if sl[i] == nil {
			k = ""
		} else {
			k = key(sl[i])
		}

		if _, found := m[k]; found {
			return true
		}

		m[k] = struct{}{}
	}

	return false
}

func FilterSlice(a, b interface{}, f func(interface{}, interface{}) bool) []interface{} {
	as := makeInterfaceSlice(a)
	if as == nil {
		return nil
	}
	bs := makeInterfaceSlice(b)
	if bs == nil {
		return nil
	}

	var n []interface{} // nolint:prealloc
	for i := range as {
		var found bool
		for j := range bs {
			if f(as[i], bs[j]) {
				found = true

				break
			}
		}

		if found {
			continue
		}

		n = append(n, as[i])
	}

	return n
}

func makeInterfaceSlice(s interface{}) []interface{} {
	v := reflect.ValueOf(s)
	switch v.Kind() {
	case reflect.Slice, reflect.Array:
		l := make([]interface{}, v.Len())
		for i := 0; i < v.Len(); i++ {
			l[i] = v.Index(i).Interface()
		}

		return l
	default:
		return nil
	}
}
