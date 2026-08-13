"""First-turn language detection (US-3.1) and conservative mid-call switching."""

from __future__ import annotations

from agent_runtime.language import detect, should_switch
from agent_runtime.models import Locale

# Fixture suite: the >=95% target is measured on this list (grows with real calls).
_FIXTURES: list[tuple[str, Locale]] = [
    ("Hi, I'd like to book an appointment for my water heater", Locale.EN_CA),
    ("Hello, my drain is blocked, can someone come today?", Locale.EN_CA),
    ("Hey there, do you do faucet repair?", Locale.EN_CA),
    ("Good morning, I need help with no hot water", Locale.EN_CA),
    ("Yes hello, I want to leave a message for the owner", Locale.EN_CA),
    ("Thanks for picking up, my basement tap leaks", Locale.EN_CA),
    ("I would like to schedule a technician this week", Locale.EN_CA),
    ("Can you call me back about a repair?", Locale.EN_CA),
    ("Hi, how much do you charge for drain cleaning?", Locale.EN_CA),
    ("Hello, is this the plumber? I need an appointment", Locale.EN_CA),
    ("Bonjour, je voudrais prendre un rendez-vous", Locale.FR_CA),
    ("Allô, mon chauffe-eau est brisé, pouvez-vous venir?", Locale.FR_CA),
    ("Bonjour, j'ai besoin d'un plombier cette semaine", Locale.FR_CA),
    ("Salut, mon drain est bouché, c'est urgent", Locale.FR_CA),
    ("Bonjour, je veux laisser un message au propriétaire", Locale.FR_CA),
    ("Est-ce que vous faites la réparation de robinet?", Locale.FR_CA),
    ("Bonjour, combien ça coûte pour déboucher un drain?", Locale.FR_CA),
    ("J'aimerais qu'on me rappelle au sujet d'une réparation", Locale.FR_CA),
    ("Bonjour, il n'y a plus d'eau chaude chez moi", Locale.FR_CA),
    ("Merci de répondre, j'ai un problème de plomberie", Locale.FR_CA),
]


def test_first_turn_accuracy_at_least_95_percent() -> None:
    hits = sum(1 for text, expected in _FIXTURES if detect(text)[0] is expected)
    assert hits / len(_FIXTURES) >= 0.95


def test_digits_only_falls_back_to_default() -> None:
    locale, confidence = detect("H2X 1Y4", default=Locale.FR_CA)
    assert locale is Locale.FR_CA
    assert confidence == 0.0


def test_switch_requires_clear_signal() -> None:
    # A single borrowed word must not flip the call locale.
    assert should_switch(Locale.EN_CA, "It's at the rendez-vous point") is None
    # A clearly French sentence does.
    assert (
        should_switch(Locale.EN_CA, "Finalement je préfère continuer en français, merci")
        is Locale.FR_CA
    )
    # Same-language text never "switches".
    assert should_switch(Locale.FR_CA, "Oui c'est ça, merci beaucoup") is None
