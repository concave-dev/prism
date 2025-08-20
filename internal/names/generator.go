// Package names provides comprehensive thematic name generation for Prism cluster nodes
// in distributed orchestration environments, delivering human-readable identifiers that
// enhance cluster management and operational visibility.
//
// This package implements a robust naming system that generates memorable node identifiers
// in Docker-style "adjective-noun" format, drawing from diverse thematic vocabularies to
// ensure both uniqueness and semantic meaning. The naming system supports the operational
// requirement for easily identifiable nodes in complex distributed deployments.
//
// THEMATIC VOCABULARY SOURCES:
// The generator combines multiple themed word collections to create distinctive names:
//
//   - General Descriptive: Docker-inspired adjectives for familiar naming patterns
//   - Space/Astronomical: Cosmic terminology for nodes handling compute workloads
//   - Mythology: Legendary references for memorable identification in large clusters
//   - Technology/Computing: Technical terms aligned with the orchestration domain
//   - Elements/Minerals: Scientific nomenclature for systematic node classification
//   - Optical/Photographic: Visual and photographic terms for clarity-focused naming
//
// NAME GENERATION STRATEGY:
// Uses secure random selection for unpredictable name patterns. Implements collision
// detection for bulk generation scenarios while maintaining performance for single requests.
//
// OPERATIONAL INTEGRATION:
// Generated names serve as primary node identifiers in cluster communications, logging,
// and administrative interfaces. The human-readable format reduces operational overhead
// by enabling intuitive node recognition compared to UUID-based identification systems.
//
// Examples: "cosmic-dragon", "quantum-nebula", "divine-titanium", "neural-phoenix", "focused-lens", "prismatic-ray"

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
	"elegant", "eloquent", "exciting", "fervent",
	"festive", "flamboyant", "friendly", "frosty",
	"funny", "gallant", "gifted", "goofy", "gracious",
	"great", "happy", "hardcore", "heuristic", "hopeful",
	"hungry", "infallible", "inspiring", "intelligent",
	"jolly", "jovial", "keen", "kind", "laughing",
	"loving", "lucid", "magical", "mystifying", "modest",
	"musing", "naughty", "nervous", "nice", "nifty",
	"nostalgic", "objective", "optimistic", "peaceful", "pedantic",
	"pensive", "practical", "priceless", "quirky", "quizzical",
	"recursing", "relaxed", "reverent", "romantic",
	"serene", "silly", "sleepy", "stoic",
	"strange", "stupefied", "suspicious", "sweet", "tender",
	"thirsty", "trusting", "unruffled", "upbeat", "vibrant",
	"vigilant", "vigorous", "wizardly", "wonderful", "xenodochial",
	"youthful", "zealous", "zen",

	// Space/Astronomical adjectives
	"cosmic", "stellar", "galactic", "lunar", "solar",
	"nebular", "orbital", "celestial", "astral", "interstellar",
	"supernova", "radiant", "infinite", "dark",
	"void", "gravitational",

	// Mythology adjectives
	"divine", "mythic", "legendary", "heroic", "ancient",
	"immortal", "eternal", "sacred", "mystical", "arcane",
	"fabled", "blessed", "cursed", "prophetic",
	"titanesque", "olympian", "nordic", "runic", "ethereal",

	// Tech/Computing adjectives
	"digital", "quantum", "cyber", "neural", "atomic",
	"virtual", "synthetic", "algorithmic", "encrypted", "networked",
	"distributed", "parallel", "concurrent", "recursive", "binary",
	"hexadecimal", "scalable", "optimized", "compiled", "dynamic",

	// Elements/Minerals adjectives
	"crystalline", "metallic", "precious", "rare", "pure",
	"refined", "alloy", "composite", "molecular",
	"radioactive", "stable", "reactive", "inert", "conductive",
	"magnetic", "transparent", "opaque", "lustrous", "malleable",

	// Optical/Photographic adjectives
	"focused", "refracted", "reflected", "prismatic", "luminous",
	"brilliant", "clear", "wide", "telephoto",
	"macro", "panoramic", "filtered", "exposed", "developed",
	"chromatic", "monochrome", "saturated", "vivid", "crisp",
	"aberrant", "achromatic", "aspheric", "birefringent", "collimated",
	"coherent", "convergent", "divergent", "diffused", "dispersive",
	"fluorescent", "holographic", "infrared", "interferometric", "iridescent",
	"isotropic", "laser", "magnified", "microscopic", "mirrored",
	"optical", "orthoscopic", "parabolic", "photonic", "polarized",
	"refractive", "spherical", "spectral", "telescopic", "ultraviolet",
	"wavelength", "zoomed", "calibrated", "corrected",
	"anamorphic", "anastigmatic", "antireflective", "apochromatic", "axial",
	"catoptric", "catadioptric", "dioptric", "elliptical", "gaussian",
	"hyperbolic", "immersive", "interferential", "multispectral", "nonlinear",
	"optoelectronic", "photorefractive", "planar", "radial", "spheroidal",
	"telecentric", "toric", "varifocal", "vignetting", "wavefront",
	"aspherical", "coaxial", "compound", "concentric", "confocal",
	"conjugate", "dispersed", "distorted", "equiconvex", "equiconcave",
	"gradient", "homogeneous", "immersion", "infinity", "marginal",
	"meniscoid", "paraxial", "peripheral", "principal", "sagittal",
	"tangential", "thick", "thin", "tilted", "zonal",
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
	"neptune", "pluto", "hubble", "voyager",

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
	"parser", "lexer", "debugger", "profiler",
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

	// Optical/Photographic equipment and concepts
	"canvas", "convex", "concave", "lens", "ray", "light",
	"prism", "mirror", "filter", "shutter", "aperture",
	"viewfinder", "sensor", "flash", "tripod",
	"focus", "exposure", "frame", "image", "pixel",
	"spectrum", "refraction", "reflection", "diffraction", "polarizer",
	"aberration", "achromat", "adapter", "amplifier", "analyzer",
	"beam", "binocular", "camera", "condenser",
	"dichroic", "diode", "eyepiece", "fiber", "grating",
	"hologram", "illuminator", "interferometer", "laser", "loupe",
	"magnifier", "microscope", "monochromator", "objective", "ocular",
	"periscope", "photocell", "photodiode", "photometer", "projector",
	"rangefinder", "reflector", "retina", "scanner", "scope",
	"spectrometer", "splitter", "telescope", "viewer", "wavelength",
	"beamsplitter", "collimation", "detector", "emitter",
	"goniometer", "interferogram", "kaleidoscope", "luminescence", "monocle",
	"optic", "resonator",
	"scintillator", "spectrograph", "stereoscope", "telecentric", "ultrascope",
	"videoscope", "waveguide", "xenon",
	"anamorphic", "biconvex", "biconcave", "cylindrical", "doublet",
	"fisheye", "fresnel", "meniscus", "planoconvex", "planoconcave",
	"telephoto", "triplet", "varifocal", "wideangle", "zeiss",
	"astigmat", "chromat", "compensator", "corrector",
	"curvature", "diopter", "element", "field",
	"focal", "gradient", "index", "medium", "nodal",
	"optics", "power", "pupil", "radius", "surface",
	"thickness", "transmission", "vertex", "zone", "spheroid",
}

// Generate creates a random Docker-style name in "adjective-noun" format from
// thematic word collections. Provides the primary interface for single node name
// generation in cluster operations.
//
// Combines randomly selected adjectives and nouns from diverse thematic vocabularies
// to create memorable, human-readable node identifiers. Returns names suitable for
// immediate use as cluster node identifiers in orchestration systems.
//
// Returns names in the format: "adjective-noun" (e.g., "cosmic-dragon", "neural-phoenix")
func Generate() string {
	adjective := adjectives[randomIndex(len(adjectives))]
	noun := nouns[randomIndex(len(nouns))]
	return fmt.Sprintf("%s-%s", adjective, noun)
}

// randomIndex generates a random index within the specified range using crypto/rand
// for unpredictable selection. Provides the core randomization primitive for name
// generation with fallback mechanisms for reliable operation.
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

// GenerateMany creates multiple unique node names with collision detection for bulk
// operations in cluster provisioning and testing scenarios. Tracks generated names
// to ensure uniqueness within the requested batch.
//
// Uses retry mechanisms with bounded attempts (100 max) to handle vocabulary exhaustion
// gracefully while maintaining performance. Prevents infinite loops when vocabulary
// space approaches exhaustion in large name generation requests.
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
