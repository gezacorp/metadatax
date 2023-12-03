![license](http://img.shields.io/badge/license-MIT-orange.svg)

metadatax is a Go library which provides a generic interface and various implementations to gather metadata from different environments.

## Installation

```bash
go get github.com/gezacorp/metadatax
```

## Usage

```go
package main

import (
    "fmt"

    "github.com/gezacorp/metadatax"
)

func main() {
    meta := metadatax.New()
    meta.AddLabel("name", "test entity")
    meta.Segment("label").AddLabel("version", "0.0.1")
    meta.Segment("image").
        AddLabel("name", "nginx").
        AddLabel("hash", "sha256:b26544c7942a085ec5c8ebaa149e6015100b0906d5b395903b5b035f6d231d35")

    for _, label := range meta.GetLabelsSlice() {
        fmt.Printf("%s = %s\n", label.Name, label.Value)
    }
}
```

### output

```text
image:hash = sha256:b26544c7942a085ec5c8ebaa149e6015100b0906d5b395903b5b035f6d231d35
image:name = nginx
label:version = 0.0.1
name = test entity
```
