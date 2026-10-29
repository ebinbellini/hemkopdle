/* ---------------------------------------------------------------- *
 * Dagens vara                                                      *
 * The page is rendered by the server (see server/templates); this   *
 * script only adds the gameplay on top of it.                       *
 * ---------------------------------------------------------------- */

const STORAGE_PREFIX = "hemkopdle:daily:";

// The number of guesses is owned by the Go code and handed to the page as a
// data attribute, so it only has to be changed in one place.
let MAX_GUESSES = 6;

const WARMTH_LABELS = {
    warm: "Varmt",
    cold: "Kallt",
};

// Older servers (and games saved before the scoring was simplified) may still
// use the six level scale. Fold those into warm/cold so a label never goes
// missing.
const LEGACY_WARMTH = {
    burning: "warm",
    hot: "warm",
    lukewarm: "warm",
    icecold: "cold",
};

function normalize_warmth(warmth) {
    if (warmth === "warm" || warmth === "cold" || warmth === "correct") {
        return warmth;
    }
    return LEGACY_WARMTH[warmth] || "cold";
}

// Up arrow when the price is higher than the guess, down arrow when it is
// lower. Red for a warm guess, blue for a cold one.
function guess_arrow(guess) {
    if (guess.result !== "low" && guess.result !== "high") {
        return null;
    }

    const arrow = document.createElement("span");
    arrow.className = "guess-arrow " + guess.warmth;
    arrow.textContent = guess.result === "low" ? "↑" : "↓";
    arrow.setAttribute("aria-label", guess.result === "low" ? "Priset är högre" : "Priset är lägre");
    return arrow;
}

const game = {
    date: null,
    product: null,
    guesses: [],
    solved: false,
    revealed: false,
    answer: null,
    busy: false,
};

const els = {};

function cache_elements() {
    els.status = document.getElementById("game-status");
    els.card = document.getElementById("product-card");
    els.image = document.getElementById("product-image");
    els.name = document.getElementById("product-name");
    els.brand = document.getElementById("product-brand");
    els.amount = document.getElementById("product-amount");
    els.form = document.getElementById("guess-form");
    els.input = document.getElementById("guess-input");
    els.submit = document.getElementById("guess-submit");
    els.error = document.getElementById("guess-error");
    els.attempts = document.getElementById("attempts");
    els.feedback = document.getElementById("feedback");
    els.history = document.getElementById("guess-history");
    els.answer = document.getElementById("answer");
}

function bind_events() {
    if (!els.form) {
        return;
    }
    els.form.addEventListener("submit", (event) => {
        event.preventDefault();
        submit_guess(els.input.value);
    });
}

/* ---------------------------- API -------------------------------- */

async function load_daily() {
    try {
        const response = await fetch("/api/daily", { headers: { Accept: "application/json" } });
        if (!response.ok) {
            throw new Error("HTTP " + response.status);
        }
        const data = await response.json();
        apply_product(data);
    } catch (error) {
        set_status("Kunde inte hämta dagens vara. Försöker igen…");
        setTimeout(load_daily, 5000);
    }
}

async function fetch_answer() {
    try {
        const response = await fetch("/api/daily/answer", { headers: { Accept: "application/json" } });
        if (!response.ok) {
            return;
        }
        game.answer = await response.json();
        save_state();
    } catch (error) {
        // The answer is displayed when the request succeeds; nothing else to do.
    }
}

/* --------------------------- Rendering --------------------------- */

// Reads what the server rendered into the page and starts the game on top of it.
function init_daily() {
    const root = document.getElementById("daily-game");
    if (!root) {
        return;
    }

    const maxGuesses = Number(root.dataset.maxGuesses);
    if (Number.isInteger(maxGuesses) && maxGuesses > 0) {
        MAX_GUESSES = maxGuesses;
    }

    if (!root.dataset.date) {
        // The server had no product to render yet (the scraper may still be
        // running), so fetch one and fill in the card when it arrives.
        load_daily();
        return;
    }

    restore_state(root.dataset.date);
    start_game();
}

// Restores the guesses for one day from localStorage.
function restore_state(date) {
    game.date = date;
    game.guesses = [];
    game.solved = false;
    game.revealed = false;
    game.answer = null;

    const saved = read_saved_state(date);
    if (!saved) {
        return;
    }

    game.guesses = Array.isArray(saved.guesses) ? saved.guesses : [];
    // Normalise guesses stored by an older version of the game.
    game.guesses.forEach((guess) => {
        guess.warmth = normalize_warmth(guess.warmth);
    });
    game.solved = Boolean(saved.solved);
    game.revealed = Boolean(saved.revealed) || (!game.solved && game.guesses.length >= MAX_GUESSES);
    game.answer = saved.answer || null;
}

// Shows the blocks that only make sense once there is a product to guess on.
function start_game() {
    set_status("");
    els.card.hidden = false;
    els.form.hidden = false;
    els.attempts.hidden = false;

    render();

    if (is_game_over()) {
        if (!game.answer) {
            fetch_answer().then(render);
        }
    } else {
        els.input.focus();
    }
}

// Fallback for a page rendered before the scraper had produced a product.
function apply_product(data) {
    game.product = data.product;

    els.image.src = data.product.image_url || "";
    els.image.alt = data.product.name || "Dagens vara";
    els.name.textContent = data.product.name || "";
    els.brand.textContent = data.product.brand || "";
    els.brand.hidden = !data.product.brand;
    els.amount.textContent = data.product.amount || "";
    els.amount.hidden = !data.product.amount;

    restore_state(data.date);
    start_game();
}

function set_status(message) {
    if (!els.status) {
        return;
    }
    els.status.textContent = message;
    els.status.hidden = message === "";
}

function render() {
    render_attempts();
    render_history();
    render_feedback();
    render_answer();

    const over = is_game_over();
    els.form.hidden = over;
    els.input.disabled = over || game.busy;
    els.submit.disabled = over || game.busy;
    els.submit.textContent = game.busy ? "Gissar…" : "Gissa";
}

function render_attempts() {
    els.attempts.textContent = "";
    for (let index = 0; index < MAX_GUESSES; index++) {
        const dot = document.createElement("span");
        dot.className = "attempt-dot";
        const guess = game.guesses[index];
        if (guess) {
            dot.classList.add("used", "warmth-" + guess.warmth);
        }
        els.attempts.appendChild(dot);
    }
}

function render_history() {
    els.history.textContent = "";
    game.guesses.forEach((guess, index) => {
        const item = document.createElement("li");
        item.className = "guess-item warmth-" + guess.warmth;

        const number = document.createElement("span");
        number.className = "guess-number";
        number.textContent = String(index + 1);

        const value = document.createElement("span");
        value.className = "guess-value";
        value.textContent = format_kr(guess.guess);

        const hint = document.createElement("span");
        hint.className = "guess-hint";
        hint.textContent = describe_guess(guess);

        const arrow = guess_arrow(guess);
        if (arrow) {
            item.append(number, arrow, value, hint);
        } else {
            item.append(number, value, hint);
        }
        els.history.appendChild(item);
    });
}

function render_feedback() {
    const last = game.guesses[game.guesses.length - 1];
    if (!last || is_game_over()) {
        els.feedback.hidden = true;
        els.feedback.textContent = "";
        return;
    }

    const direction = document.createElement("strong");
    const detail = document.createElement("span");
    detail.className = "feedback-text";
    const warmthLabel = WARMTH_LABELS[last.warmth] || "";

    if (last.result === "low") {
        direction.textContent = "För lågt";
        detail.textContent = join_parts(warmthLabel, "priset är högre.");
    } else {
        direction.textContent = "För högt";
        detail.textContent = join_parts(warmthLabel, "priset är lägre.");
    }

    const arrow = guess_arrow(last);

    els.feedback.className = "feedback warmth-" + last.warmth;
    els.feedback.textContent = "";
    if (arrow) {
        els.feedback.append(arrow, direction, detail);
    } else {
        els.feedback.append(direction, detail);
    }
    els.feedback.hidden = false;
}

function render_answer() {
    if (!is_game_over() || !game.answer) {
        els.answer.hidden = true;
        els.answer.textContent = "";
        return;
    }

    const title = document.createElement("h3");
    title.textContent = game.solved
        ? "Rätt på " + game.guesses.length + " försök!"
        : "Slut på försök";

    const price = document.createElement("p");
    price.className = "answer-price";
    price.textContent = "Priset var " + format_kr(game.answer.price);

    const note = document.createElement("p");
    note.className = "answer-note";
    note.textContent = game.solved
        ? "Snyggt gissat! En ny vara väntar vid midnatt."
        : "En ny vara väntar vid midnatt. Då tar du den!";

    els.answer.className = "answer " + (game.solved ? "answer-win" : "answer-lose");
    els.answer.textContent = "";
    els.answer.append(title, price, note);
    els.answer.hidden = false;
}

/* ---------------------------- Gameplay --------------------------- */

function is_game_over() {
    return game.solved || game.revealed || game.guesses.length >= MAX_GUESSES;
}

function parse_guess(raw) {
    if (typeof raw !== "string") {
        return null;
    }

    const normalized = raw.trim().replace(/\s/g, "").replace(",", ".");
    if (normalized === "") {
        return null;
    }

    const value = Number(normalized);
    if (!Number.isFinite(value) || value <= 0) {
        return null;
    }

    return Math.round(value * 100) / 100;
}

function describe_guess(guess) {
    if (guess.result === "correct") {
        return "Rätt!";
    }
    const direction = guess.result === "low" ? "För lågt" : "För högt";
    const label = WARMTH_LABELS[guess.warmth];
    return label ? direction + " · " + label : direction;
}

function join_parts(...parts) {
    return parts.filter(Boolean).join(" - ");
}

function format_kr(value) {
    if (!Number.isFinite(value)) {
        return "";
    }
    return value.toFixed(2).replace(".", ",") + " kr";
}

function show_error(message) {
    els.error.textContent = message;
    els.error.hidden = message === "";
}

async function submit_guess(raw) {
    if (is_game_over() || game.busy) {
        return;
    }

    const guess = parse_guess(raw);
    if (guess === null) {
        show_error("Ange ett pris i kronor, till exempel 24,50.");
        return;
    }
    show_error("");

    game.busy = true;
    render();

    try {
        const response = await fetch("/api/daily/guess", {
            method: "POST",
            headers: { "Content-Type": "application/json", Accept: "application/json" },
            body: JSON.stringify({ guess: guess }),
        });

        if (!response.ok) {
            throw new Error("HTTP " + response.status);
        }

        const data = await response.json();
        game.guesses.push({
            guess: guess,
            result: data.result,
            warmth: normalize_warmth(data.warmth),
        });

        if (data.result === "correct") {
            game.solved = true;
        } else if (game.guesses.length >= MAX_GUESSES) {
            game.revealed = true;
        }

        if (is_game_over()) {
            await fetch_answer();
        }

        els.input.value = "";
        save_state();
    } catch (error) {
        show_error("Kunde inte skicka gissningen. Försök igen.");
    } finally {
        game.busy = false;
        render();
        if (!is_game_over()) {
            els.input.focus();
        }
    }
}

/* --------------------------- Persistence ------------------------- */

function read_saved_state(date) {
    try {
        const raw = window.localStorage.getItem(STORAGE_PREFIX + date);
        return raw ? JSON.parse(raw) : null;
    } catch (error) {
        return null;
    }
}

function save_state() {
    if (!game.date) {
        return;
    }
    try {
        window.localStorage.setItem(STORAGE_PREFIX + game.date, JSON.stringify({
            guesses: game.guesses,
            solved: game.solved,
            revealed: game.revealed,
            answer: game.answer,
        }));
    } catch (error) {
        // Private mode or a full quota: the game still works without storage.
    }
}

document.addEventListener("DOMContentLoaded", () => {
    cache_elements();
    bind_events();
    init_daily();
});