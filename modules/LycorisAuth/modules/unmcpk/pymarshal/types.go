package pymarshal

const (
	TypeNull            = '0' // 0x30
	TypeNone            = 'N' // 0x4e
	TypeFalse           = 'F' // 0x46
	TypeTrue            = 'T' // 0x54
	TypeStopIteration   = 'S' // 0x53
	TypeEllipsis        = '.' // 0x2e
	TypeInt             = 'i' // 0x69
	TypeInt64           = 'I' // 0x49
	TypeFloat           = 'f' // 0x66
	TypeBinaryFloat     = 'g' // 0x67
	TypeComplex         = 'x' // 0x78
	TypeBinaryComplex   = 'y' // 0x79
	TypeLong            = 'l' // 0x6c
	TypeString          = 's' // 0x73
	TypeInterned        = 't' // 0x74
	TypeStringReference = 'R' // 0x52
	TypeTuple           = '(' // 0x28
	TypeList            = '[' // 0x5b
	TypeDictionary      = '{' // 0x7b
	TypeCode            = 'c' // 0x63
	TypeUnicode         = 'u' // 0x75
	TypeUnknown         = '?' // 0x3f
	TypeSet             = '<' // 0x3c
	TypeFrozenSet       = '>' // 0x3e
)
