// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package runtime

import "testing"

func TestOwned(t *testing.T) {
 var item Inspection
 item.Config.Labels=map[string]string{"dev.orisema.managed":"true","dev.orisema.app":"demo","dev.orisema.service":"web"}
 if !Owned(item,"demo","web"){t.Fatal("expected matching ownership")}
 if Owned(item,"demo","db"){t.Fatal("unexpected service ownership")}
 if Owned(item,"other","web"){t.Fatal("unexpected application ownership")}
 delete(item.Config.Labels,"dev.orisema.managed")
 if Owned(item,"demo","web"){t.Fatal("missing managed label accepted")}
}
