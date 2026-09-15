package main

import "testing"

func TestXerarInformeErroIARexeitaMensaxeBaleira(t *testing.T) {
	a := &App{}
	_, err := a.XerarInformeErroIA(InformeErroRequest{Anaco: "algo", MensaxeErro: "   "})
	if err == nil {
		t.Fatal("agardaba un erro con MensaxeErro baleira")
	}
}
