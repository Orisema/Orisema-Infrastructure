// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package planner

import (
 "context"
 "testing"
 "github.com/Orisema/Orisema-Infrastructure/internal/manifest"
 "github.com/Orisema/Orisema-Infrastructure/internal/runtime"
)

type fakeInspector struct { item runtime.Inspection; err error }
func (f fakeInspector) Inspect(context.Context,string)(runtime.Inspection,error){return f.item,f.err}

func TestBuildInspected(t *testing.T) {
 m:=&manifest.Manifest{Name:"hello-orisema",Services:[]manifest.Service{{Name:"web",Image:"docker.io/library/nginx:1.27",Ports:[]manifest.Port{{Host:8080,Container:80}}}}}
 existing:=[]runtime.Container{{Names:[]string{"orisema-hello-orisema-web"}}}
 var item runtime.Inspection
 item.Config.Image=m.Services[0].Image
 item.Config.Labels=map[string]string{"dev.orisema.managed":"true","dev.orisema.app":"hello-orisema","dev.orisema.service":"web"}
 item.HostConfig.PortBindings=map[string][]runtime.Binding{"80/tcp":{{HostIP:"0.0.0.0",HostPort:"8080"}}}
 check:=func(want Action) {
  t.Helper()
  got:=BuildInspected(context.Background(),m,existing,fakeInspector{item:item})
  if len(got)!=1||got[0].Action!=want {t.Fatalf("want %s, got %#v",want,got)}
 }
 check(Noop)
 item.Config.Image="docker.io/library/nginx:other"
 check(Drift)
 item.Config.Image=m.Services[0].Image
 delete(item.Config.Labels,"dev.orisema.managed")
 check(Review)
 existing=nil
 check(Create)
}
