package packet

import (
	"image/color"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/go-gl/mathgl/mgl32"
)

const (
	ScriptDebugShapeLine = iota
	ScriptDebugShapeBox
	ScriptDebugShapeSphere
	ScriptDebugShapeCircle
	ScriptDebugShapeText
	ScriptDebugShapeArrow
)

const (
	debugShapePayloadNone = iota
	debugShapePayloadArrow
	debugShapePayloadText
	debugShapePayloadBox
	debugShapePayloadLine
	debugShapePayloadSphereOrCircle
)

// DebugDrawerShape defines a single debug shape to be rendered on the client.
// The wire format matches PrimitiveShapes v859 while retaining the existing
// NetEase-facing API name and packet ID.
type DebugDrawerShape struct {
	NetworkID     uint64
	Type          protocol.Optional[uint8]
	Location      protocol.Optional[mgl32.Vec3]
	Scale         protocol.Optional[float32]
	Rotation      protocol.Optional[mgl32.Vec3]
	TotalTimeLeft protocol.Optional[float32]
	Colour        protocol.Optional[color.RGBA]
	Dimension     int32

	Text             string
	BoxBound         mgl32.Vec3
	LineEndLocation  mgl32.Vec3
	ArrowEndLocation protocol.Optional[mgl32.Vec3]
	ArrowHeadLength  protocol.Optional[float32]
	ArrowHeadRadius  protocol.Optional[float32]
	Segments         byte
	ArrowSegments    protocol.Optional[byte]
}

func (x *DebugDrawerShape) Marshal(io protocol.IO) {
	io.Varuint64(&x.NetworkID)
	protocol.OptionalFunc(io, &x.Type, io.Uint8)
	protocol.OptionalFunc(io, &x.Location, io.Vec3)
	protocol.OptionalFunc(io, &x.Scale, io.Float32)
	protocol.OptionalFunc(io, &x.Rotation, io.Vec3)
	protocol.OptionalFunc(io, &x.TotalTimeLeft, io.Float32)
	protocol.OptionalFunc(io, &x.Colour, io.BEARGB)
	io.Varint32(&x.Dimension)

	payloadType := uint32(debugShapePayloadNone)
	if typ, ok := x.Type.Value(); ok {
		switch typ {
		case ScriptDebugShapeArrow:
			payloadType = debugShapePayloadArrow
		case ScriptDebugShapeText:
			payloadType = debugShapePayloadText
		case ScriptDebugShapeBox:
			payloadType = debugShapePayloadBox
		case ScriptDebugShapeLine:
			payloadType = debugShapePayloadLine
		case ScriptDebugShapeSphere, ScriptDebugShapeCircle:
			payloadType = debugShapePayloadSphereOrCircle
		}
	}
	io.Varuint32(&payloadType)

	switch payloadType {
	case debugShapePayloadNone:
	case debugShapePayloadArrow:
		protocol.OptionalFunc(io, &x.ArrowEndLocation, io.Vec3)
		protocol.OptionalFunc(io, &x.ArrowHeadLength, io.Float32)
		protocol.OptionalFunc(io, &x.ArrowHeadRadius, io.Float32)
		protocol.OptionalFunc(io, &x.ArrowSegments, io.Uint8)
	case debugShapePayloadText:
		io.String(&x.Text)
	case debugShapePayloadBox:
		io.Vec3(&x.BoxBound)
	case debugShapePayloadLine:
		io.Vec3(&x.LineEndLocation)
	case debugShapePayloadSphereOrCircle:
		io.Uint8(&x.Segments)
	default:
		io.UnknownEnumOption(payloadType, "debug shape payload type")
	}
}

// ServerScriptDebugDrawer instructs the client to render one or more debug shapes.
type ServerScriptDebugDrawer struct {
	Shapes []DebugDrawerShape
}

// ID ...
func (*ServerScriptDebugDrawer) ID() uint32 {
	return IDServerScriptDebugDrawer
}

func (pk *ServerScriptDebugDrawer) Marshal(io protocol.IO) {
	protocol.Slice(io, &pk.Shapes)
}
