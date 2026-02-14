package engine

import "github.com/danshapiro/kilroy/internal/attractor/model"

import "github.com/danshapiro/kilroy/internal/attractor/runtime"

func NewContextWithGraphAttrs(g *model.Graph) *runtime.Context {
	ctx := runtime.NewContext()
	if g == nil {
		return ctx
	}
	for k, v := range g.Attrs {
		ctx.Set("graph."+k, v)
	}
	ctx.Set("graph.goal", g.Attrs["goal"])
	return ctx
}
