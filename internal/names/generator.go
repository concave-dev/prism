// Package names provides thematic name generation for Prism nodes.
// Generates human-readable names in the format "adjective-noun" from multiple themes:
// animals, space/astronomy, mythology, tech/computing, and elements/minerals.
// Examples: "cosmic-dragon", "quantum-nebula", "divine-titanium", "neural-phoenix"
package names

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// Adjectives from multiple themes for name generation
var adjectives = []string{
	// General/Docker-like adjectives
	"admiring", "adoring", "affectionate", "agitated", "amazing",
	"angry", "awesome", "beautiful", "blissful", "bold",
	"boring", "brave", "busy", "charming", "clever",
	"cool", "compassionate", "competent", "confident",
	"cranky", "crazy", "dazzling", "determined", "distracted",
	"dreamy", "eager", "ecstatic", "elastic", "elated",
	"elegant", "eloquent", "epic", "exciting", "fervent",
	"festive", "flamboyant", "focused", "friendly", "frosty",
	"funny", "gallant", "gifted", "goofy", "gracious",
	"great", "happy", "hardcore", "heuristic", "hopeful",
	"hungry", "infallible", "inspiring", "intelligent",
	"jolly", "jovial", "keen", "kind", "laughing",
	"loving", "lucid", "magical", "mystifying", "modest",
	"musing", "naughty", "nervous", "nice", "nifty",
	"nostalgic", "objective", "optimistic", "peaceful", "pedantic",
	"pensive", "practical", "priceless", "quirky", "quizzical",
	"recursing", "relaxed", "reverent", "romantic",
	"serene", "sharp", "silly", "sleepy", "stoic",
	"strange", "stupefied", "suspicious", "sweet", "tender",
	"thirsty", "trusting", "unruffled", "upbeat", "vibrant",
	"vigilant", "vigorous", "wizardly", "wonderful", "xenodochial",
	"youthful", "zealous", "zen",

	// Space/Astronomical adjectives
	"cosmic", "stellar", "galactic", "lunar", "solar",
	"nebular", "orbital", "celestial", "astral", "interstellar",
	"supernova", "radiant", "luminous", "infinite", "dark",
	"void", "plasma", "magnetic", "gravitational", "binary",

	// Mythology adjectives
	"divine", "mythic", "legendary", "heroic", "ancient",
	"immortal", "eternal", "sacred", "mystical", "arcane",
	"epic", "fabled", "blessed", "cursed", "prophetic",
	"titanesque", "olympian", "nordic", "runic", "ethereal",

	// Tech/Computing adjectives
	"digital", "quantum", "cyber", "neural", "atomic",
	"virtual", "synthetic", "algorithmic", "encrypted", "networked",
	"distributed", "parallel", "concurrent", "recursive", "binary",
	"hexadecimal", "scalable", "optimized", "compiled", "dynamic",

	// Elements/Minerals adjectives
	"crystalline", "metallic", "precious", "rare", "pure",
	"refined", "alloy", "composite", "molecular", "atomic",
	"radioactive", "stable", "reactive", "inert", "conductive",
	"magnetic", "transparent", "opaque", "lustrous", "malleable",
}

// Nouns from multiple themes for name generation
var nouns = []string{
	// Animals
	"albatross", "ant", "antelope", "ape", "armadillo",
	"badger", "bat", "bear", "bee", "beetle",
	"bird", "bison", "boar", "buffalo", "butterfly",
	"camel", "cat", "caterpillar", "cheetah", "chicken",
	"chipmunk", "cobra", "cow", "coyote", "crab",
	"crane", "crocodile", "crow", "deer", "dinosaur",
	"dog", "dolphin", "donkey", "dove", "dragonfly",
	"duck", "eagle", "elephant", "elk", "emu",
	"falcon", "ferret", "finch", "fish", "flamingo",
	"fly", "fox", "frog", "giraffe", "gnat",
	"gnu", "goat", "goose", "gorilla", "grasshopper",
	"hamster", "hare", "hawk", "hedgehog", "heron",
	"hippo", "hornet", "horse", "hummingbird", "hyena",
	"iguana", "jackal", "jaguar", "jay", "kangaroo",
	"kitten", "koala", "lamb", "leopard", "lion",
	"lizard", "llama", "lobster", "lynx", "magpie",
	"mammoth", "manatee", "meerkat", "mole", "mongoose",
	"monkey", "moose", "mosquito", "mouse", "mule",
	"nightingale", "octopus", "opossum", "orangutan", "orca",
	"ostrich", "otter", "owl", "ox", "oyster",
	"panda", "panther", "parrot", "pelican", "penguin",
	"pig", "pigeon", "porcupine", "porpoise", "quail",
	"rabbit", "raccoon", "ram", "rat", "raven",
	"rhino", "robin", "rooster", "seal", "shark",
	"sheep", "skunk", "sloth", "snail", "snake",
	"sparrow", "spider", "squirrel", "starfish", "stingray",
	"stork", "swallow", "swan", "tiger", "toad",
	"trout", "turkey", "turtle", "walrus", "wasp",
	"weasel", "whale", "wildcat", "wolf", "wombat",
	"woodpecker", "worm", "yak", "zebra",

	// Space/Astronomical objects
	"nebula", "quasar", "pulsar", "comet", "meteor",
	"galaxy", "supernova", "blackhole", "star", "planet",
	"asteroid", "satellite", "moon", "sun", "universe",
	"cosmos", "void", "plasma", "photon", "neutron",
	"orbit", "eclipse", "constellation", "andromeda", "milkyway",
	"venus", "mars", "jupiter", "saturn", "uranus",
	"neptune", "pluto", "apollo", "hubble", "voyager",

	// Mythology creatures and objects
	"titan", "phoenix", "dragon", "griffin", "kraken",
	"hydra", "sphinx", "centaur", "minotaur", "cyclops",
	"pegasus", "unicorn", "chimera", "banshee", "valkyrie",
	"thor", "odin", "loki", "zeus", "athena",
	"apollo", "artemis", "hermes", "poseidon", "hades",
	"anubis", "osiris", "isis", "ra", "thoth",
	"excalibur", "mjolnir", "trident", "aegis", "pandora",

	// Tech/Computing terms
	"circuit", "processor", "matrix", "vector", "algorithm",
	"protocol", "framework", "kernel", "daemon", "thread",
	"pipeline", "buffer", "cache", "registry", "compiler",
	"parser", "scanner", "lexer", "debugger", "profiler",
	"database", "server", "client", "proxy", "gateway",
	"router", "firewall", "cluster", "node", "pod",
	"container", "microservice", "api", "endpoint", "webhook",

	// Elements and minerals
	"diamond", "platinum", "titanium", "cobalt", "silicon",
	"carbon", "hydrogen", "oxygen", "nitrogen", "phosphorus",
	"sulfur", "chlorine", "argon", "potassium", "calcium",
	"iron", "nickel", "copper", "zinc", "silver",
	"gold", "mercury", "lead", "uranium", "plutonium",
	"crystal", "quartz", "ruby", "emerald", "sapphire",
	"topaz", "amethyst", "garnet", "jade", "opal",
}

// Generate generates a random Docker-like name in "adjective-noun" format
func Generate() string {
	adjective := adjectives[randomIndex(len(adjectives))]
	noun := nouns[randomIndex(len(nouns))]
	return fmt.Sprintf("%s-%s", adjective, noun)
}

// randomIndex generates a cryptographically secure random index
func randomIndex(max int) int {
	if max <= 0 {
		return 0
	}

	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		// Fallback to a simple index if crypto/rand fails
		return 0
	}

	return int(n.Int64())
}

// GenerateMany generates multiple unique names, useful for testing or bulk operations
func GenerateMany(count int) []string {
	if count <= 0 {
		return []string{}
	}

	names := make([]string, count)
	used := make(map[string]bool)

	for i := 0; i < count; i++ {
		var name string
		attempts := 0

		// Try to generate a unique name, with a reasonable retry limit
		for {
			name = Generate()
			if !used[name] || attempts > 100 {
				break
			}
			attempts++
		}

		used[name] = true
		names[i] = name
	}

	return names
}
