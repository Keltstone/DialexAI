package scan

import "testing"

func TestDeriveKind(t *testing.T) {
	cases := []struct {
		lang Lang
		key  string
		sig  string
		want string
	}{
		// Go
		{Go, "Hello", "func Hello() string", "func"},
		{Go, "Box.Get", "func (b *Box) Get() string", "method"},
		{Go, "MyStruct", "type MyStruct struct { Field int }", "class"},
		{Go, "Reader", "type Reader interface { Read() }", "interface"},
		{Go, "CustomInt", "type CustomInt int", "type"},
		{Go, "MAX", "const MAX = 100", "const"},
		{Go, "count", "var count int", "var"},

		// Kotlin
		{Kotlin, "foo", "fun foo()", "func"},
		{Kotlin, "MyClass.bar", "fun bar()", "method"},
		{Kotlin, "MyClass", "class MyClass", "class"},
		{Kotlin, "DataClass", "data class DataClass(val x: Int)", "class"},
		{Kotlin, "MyObject", "object MyObject", "object"},
		{Kotlin, "Service", "interface Service", "interface"},
		{Kotlin, "MyAlias", "typealias MyAlias = String", "type"},
		{Kotlin, "PROP", "const val PROP = 1", "const"},
		{Kotlin, "prop", "val prop: Int", "property"},
		{Kotlin, "propVar", "var propVar: String", "property"},
		{Kotlin, "MyClass.<init>", "constructor(x: Int)", "constructor"},
		{Kotlin, "Color.RED", "RED", "enum_entry"},

		// Java
		{Java, "MyClass", "public class MyClass", "class"},
		{Java, "MyInterface", "public interface MyInterface", "interface"},
		{Java, "Color", "public enum Color", "enum"},
		{Java, "Point", "public record Point(int x, int y)", "record"},
		{Java, "MyClass.doWork", "public void doWork()", "method"},
		{Java, "MyClass.<init>", "public MyClass()", "constructor"},
		{Java, "MyClass.field", "public int field", "field"},
		{Java, "Color.RED", "RED", "enum_entry"},

		// TS
		{TS, "run", "export function run()", "func"},
		{TS, "Worker", "export class Worker", "class"},
		{TS, "Config", "export interface Config", "interface"},
		{TS, "MyType", "export type MyType = string", "type"},
		{TS, "Status", "export enum Status", "enum"},
		{TS, "VERSION", "export const VERSION = '1.0.0'", "const"},
		{TS, "count", "export let count = 0", "var"},
		{TS, "Worker.run", "run(): void", "method"},
		{TS, "Worker.id", "id: number", "property"},
		{TS, "Worker.constructor", "constructor(name: string)", "constructor"},
		{TS, "Status.Active", "Active = 1", "enum_entry"},

		// Python
		{Python, "process", "def process():", "func"},
		{Python, "Handler.handle", "def handle(self):", "method"},
		{Python, "Handler", "class Handler:", "class"},
		{Python, "MAX_RETRIES", "MAX_RETRIES = 5", "const"},
		{Python, "timeout", "timeout = 30", "var"},
	}

	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			got := DeriveKind(tc.lang, tc.key, tc.sig)
			if got != tc.want {
				t.Errorf("DeriveKind(%v, %q, %q) = %q, want %q", tc.lang, tc.key, tc.sig, got, tc.want)
			}
		})
	}
}
