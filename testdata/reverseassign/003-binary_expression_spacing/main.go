// <<<<< reverseassign,8,3,8,10,fail
package main

import "fmt"
import "reflect"

func main() {
  var i int=3+4.5
  fmt.Println("type of i is", reflect.TypeOf(i))
  fmt.Println("value of i is", i)
}