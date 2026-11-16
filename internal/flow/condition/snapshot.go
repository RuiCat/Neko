// snapshot.go fixes the read-only snapshot surface and the kernel paths the
// judge reads. The interface deliberately exposes Get only: judgment has no
// write method to call.
package condition

import "fmt"

// Snapshot is the minimal read-only entity view consumed by the judge. The
// caller supplies a projection that also exposes the item view under
// PATH_ITEMS; the judge never mutates it.
type Snapshot interface {
	Get(path string) (any, error)
}

// Snapshot paths. Scene, money and affinity mirror the kernel models; the item
// view and the time placeholder are K2-side conventions documented here.
const (
	// PATH_SCENE is the actor's current scene (engine reads 位置.场景).
	PATH_SCENE = "位置.场景"
	// PATH_MONEY is the actor's currency (trade kernel 交易.货币).
	PATH_MONEY = "交易.货币"
	// PATH_AFFINITY is one relation entity's 关系.数值.好感度.
	PATH_AFFINITY = "关系.数值.好感度"
	// PATH_ITEMS is the read-only item view: either a list of item records or
	// a map from item name to an item record, each carrying 名称/所属场景/数量.
	PATH_ITEMS = "素材.物品"
	// PATH_TIME is the placeholder path for a future day/time source. No
	// kernel field exists yet, so a time requirement is always not_ready.
	PATH_TIME = "时间"
)

// readString reads one scalar string snapshot path.
func readString(snap Snapshot, path string) (string, error) {
	raw, err := snap.Get(path)
	if err != nil {
		return "", err
	}
	text, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s: want string, got %T", path, raw)
	}
	return text, nil
}
