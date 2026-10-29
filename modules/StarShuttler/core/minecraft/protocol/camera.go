package protocol

import (
	"image/color"

	"github.com/go-gl/mathgl/mgl32"
)

const (
	AimAssistTargetModeAngle = iota
	AimAssistTargetModeDistance
)

const (
	AudioListenerCamera = iota
	AudioListenerPlayer
)

const (
	EasingTypeLinear = iota
	EasingTypeSpring
	EasingTypeInQuad
	EasingTypeOutQuad
	EasingTypeInOutQuad
	EasingTypeInCubic
	EasingTypeOutCubic
	EasingTypeInOutCubic
	EasingTypeInQuart
	EasingTypeOutQuart
	EasingTypeInOutQuart
	EasingTypeInQuint
	EasingTypeOutQuint
	EasingTypeInOutQuint
	EasingTypeInSine
	EasingTypeOutSine
	EasingTypeInOutSine
	EasingTypeInExpo
	EasingTypeOutExpo
	EasingTypeInOutExpo
	EasingTypeInCirc
	EasingTypeOutCirc
	EasingTypeInOutCirc
	EasingTypeInBounce
	EasingTypeOutBounce
	EasingTypeInOutBounce
	EasingTypeInBack
	EasingTypeOutBack
	EasingTypeInOutBack
	EasingTypeInElastic
	EasingTypeOutElastic
	EasingTypeInOutElastic
)

const (
	SplineEaseTypeCatmullRom = iota
	SplineEaseTypeLinear
)

// CameraEase represents an easing function that can be used by a CameraInstructionSet.
type CameraEase struct {
	Type     uint8
	Duration float32
}

func (x *CameraEase) Marshal(r IO) {
	r.Uint8(&x.Type)
	r.Float32(&x.Duration)
}

// CameraInstructionSet represents a camera instruction that sets the camera to a specified preset.
type CameraInstructionSet struct {
	Preset                        uint32
	Ease                          Optional[CameraEase]
	Position                      Optional[mgl32.Vec3]
	Rotation                      Optional[mgl32.Vec2]
	Facing                        Optional[mgl32.Vec3]
	ViewOffset                    Optional[mgl32.Vec2]
	EntityOffset                  Optional[mgl32.Vec3]
	Default                       Optional[bool]
	IgnoreStartingValuesComponent bool
}

func (x *CameraInstructionSet) Marshal(r IO) {
	r.Uint32(&x.Preset)
	OptionalMarshaler(r, &x.Ease)
	OptionalFunc(r, &x.Position, r.Vec3)
	OptionalFunc(r, &x.Rotation, r.Vec2)
	OptionalFunc(r, &x.Facing, r.Vec3)
	OptionalFunc(r, &x.ViewOffset, r.Vec2)
	OptionalFunc(r, &x.EntityOffset, r.Vec3)
	OptionalFunc(r, &x.Default, r.Bool)
	r.Bool(&x.IgnoreStartingValuesComponent)
}

type CameraFadeTimeData struct {
	FadeInDuration  float32
	WaitDuration    float32
	FadeOutDuration float32
}

func (x *CameraFadeTimeData) Marshal(r IO) {
	r.Float32(&x.FadeInDuration)
	r.Float32(&x.WaitDuration)
	r.Float32(&x.FadeOutDuration)
}

type CameraInstructionFade struct {
	TimeData Optional[CameraFadeTimeData]
	Colour   Optional[color.RGBA]
}

func (x *CameraInstructionFade) Marshal(r IO) {
	OptionalMarshaler(r, &x.TimeData)
	OptionalFunc(r, &x.Colour, r.RGB)
}

type CameraInstructionTarget struct {
	CenterOffset   Optional[mgl32.Vec3]
	EntityUniqueID int64
}

func (x *CameraInstructionTarget) Marshal(r IO) {
	OptionalFunc(r, &x.CenterOffset, r.Vec3)
	r.Int64(&x.EntityUniqueID)
}

// CameraInstructionFieldOfView represents a camera instruction that updates the field of view.
type CameraInstructionFieldOfView struct {
	// FieldOfView is the field of view of the camera.
	FieldOfView float32
	// EaseTime is the time in seconds that the easing function should take.
	EaseTime float32
	// EaseType is the type of easing function used. This is one of the constants above.
	EaseType uint8
	// Clear can be set to true to clear the current instruction.
	Clear bool
}

// Marshal encodes/decodes a CameraInstructionFieldOfView.
func (x *CameraInstructionFieldOfView) Marshal(r IO) {
	r.Float32(&x.FieldOfView)
	r.Float32(&x.EaseTime)
	r.Uint8(&x.EaseType)
	r.Bool(&x.Clear)
}

// CameraPreset represents a basic preset that can be extended upon by more complex instructions.
type CameraPreset struct {
	Name                    string
	Parent                  string
	PosX                    Optional[float32]
	PosY                    Optional[float32]
	PosZ                    Optional[float32]
	RotX                    Optional[float32]
	RotY                    Optional[float32]
	RotationSpeed           Optional[float32]
	SnapToTarget            Optional[bool]
	HorizontalRotationLimit Optional[mgl32.Vec2]
	VerticalRotationLimit   Optional[mgl32.Vec2]
	ContinueTargeting       Optional[bool]
	TrackingRadius          Optional[float32]
	ViewOffset              Optional[mgl32.Vec2]
	EntityOffset            Optional[mgl32.Vec3]
	Radius                  Optional[float32]
	MinYawLimit             Optional[float32]
	MaxYawLimit             Optional[float32]
	AudioListener           Optional[byte]
	PlayerEffects           Optional[bool]
	AimAssist               Optional[CameraPresetAimAssist]
	ControlScheme           Optional[byte]
}

func (x *CameraPreset) Marshal(r IO) {
	r.String(&x.Name)
	r.String(&x.Parent)
	OptionalFunc(r, &x.PosX, r.Float32)
	OptionalFunc(r, &x.PosY, r.Float32)
	OptionalFunc(r, &x.PosZ, r.Float32)
	OptionalFunc(r, &x.RotX, r.Float32)
	OptionalFunc(r, &x.RotY, r.Float32)
	OptionalFunc(r, &x.RotationSpeed, r.Float32)
	OptionalFunc(r, &x.SnapToTarget, r.Bool)
	OptionalFunc(r, &x.HorizontalRotationLimit, r.Vec2)
	OptionalFunc(r, &x.VerticalRotationLimit, r.Vec2)
	OptionalFunc(r, &x.ContinueTargeting, r.Bool)
	OptionalFunc(r, &x.TrackingRadius, r.Float32)
	OptionalFunc(r, &x.ViewOffset, r.Vec2)
	OptionalFunc(r, &x.EntityOffset, r.Vec3)
	OptionalFunc(r, &x.Radius, r.Float32)
	OptionalFunc(r, &x.MinYawLimit, r.Float32)
	OptionalFunc(r, &x.MaxYawLimit, r.Float32)
	OptionalFunc(r, &x.AudioListener, r.Uint8)
	OptionalFunc(r, &x.PlayerEffects, r.Bool)
	OptionalMarshaler(r, &x.AimAssist)
	OptionalFunc(r, &x.ControlScheme, r.Uint8)
}

type CameraPresetAimAssist struct {
	Preset     Optional[string]
	TargetMode Optional[int32]
	Angle      Optional[mgl32.Vec2]
	Distance   Optional[float32]
}

func (x *CameraPresetAimAssist) Marshal(r IO) {
	OptionalFunc(r, &x.Preset, r.String)
	OptionalFunc(r, &x.TargetMode, r.Int32)
	OptionalFunc(r, &x.Angle, r.Vec2)
	OptionalFunc(r, &x.Distance, r.Float32)
}

type CameraAimAssistCategory struct {
	Name       string
	Priorities CameraAimAssistPriorities
}

func (x *CameraAimAssistCategory) Marshal(r IO) {
	r.String(&x.Name)
	Single(r, &x.Priorities)
}

type CameraAimAssistPriorities struct {
	Entities      []CameraAimAssistPriority
	Blocks        []CameraAimAssistPriority
	EntityDefault Optional[int32]
	BlockDefault  Optional[int32]
}

func (x *CameraAimAssistPriorities) Marshal(r IO) {
	Slice(r, &x.Entities)
	Slice(r, &x.Blocks)
	OptionalFunc(r, &x.EntityDefault, r.Int32)
	OptionalFunc(r, &x.BlockDefault, r.Int32)
}

type CameraAimAssistPriority struct {
	Identifier string
	Priority   int32
}

func (x *CameraAimAssistPriority) Marshal(r IO) {
	r.String(&x.Identifier)
	r.Int32(&x.Priority)
}

type CameraAimAssistPreset struct {
	Identifier          string
	BlockExclusions     []string
	LiquidTargets       []string
	ItemSettings        []CameraAimAssistItemSettings
	DefaultItemSettings Optional[string]
	HandSettings        Optional[string]
}

func (x *CameraAimAssistPreset) Marshal(r IO) {
	r.String(&x.Identifier)
	FuncSlice(r, &x.BlockExclusions, r.String)
	FuncSlice(r, &x.LiquidTargets, r.String)
	Slice(r, &x.ItemSettings)
	OptionalFunc(r, &x.DefaultItemSettings, r.String)
	OptionalFunc(r, &x.HandSettings, r.String)
}

type CameraAimAssistItemSettings struct {
	Item     string
	Category string
}

func (x *CameraAimAssistItemSettings) Marshal(r IO) {
	r.String(&x.Item)
	r.String(&x.Category)
}

// CameraRotationOption represents a rotation option for camera spline instructions.
type CameraRotationOption struct {
	// Value is the rotation value.
	Value mgl32.Vec3
	// Time is the time for this rotation option.
	Time float32
}

// Marshal encodes/decodes a CameraRotationOption.
func (x *CameraRotationOption) Marshal(r IO) {
	r.Vec3(&x.Value)
	r.Float32(&x.Time)
}

// CameraSplineInstruction represents a camera instruction that creates a spline path for the camera to follow.
type CameraSplineInstruction struct {
	// TotalTime is the total time for the spline animation.
	TotalTime float32
	// EaseType is the type of easing function used. This is one of the constants above.
	EaseType uint8
	// Curve is a list of points that define the spline curve.
	Curve []mgl32.Vec3
	// ProgressKeyFrames is a list of key frames for the progress of the spline.
	ProgressKeyFrames []mgl32.Vec2
	// RotationOptions is a list of rotation options for the spline.
	RotationOptions []CameraRotationOption
}

// Marshal encodes/decodes a CameraSplineInstruction.
func (x *CameraSplineInstruction) Marshal(r IO) {
	r.Float32(&x.TotalTime)
	r.Uint8(&x.EaseType)
	FuncSlice(r, &x.Curve, r.Vec3)
	FuncSlice(r, &x.ProgressKeyFrames, r.Vec2)
	Slice(r, &x.RotationOptions)
}
