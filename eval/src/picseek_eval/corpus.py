import math
import random
import textwrap
from collections.abc import Callable
from dataclasses import dataclass, field
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

SANS = "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"
SANS_BOLD = "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"
MONO = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"


def font(path: str, size: int) -> ImageFont.FreeTypeFont:
    return ImageFont.truetype(path, size, layout_engine=ImageFont.Layout.BASIC)


@dataclass(frozen=True)
class Shot:
    name: str
    kind: str | None
    render: Callable[[], Image.Image]


@dataclass(frozen=True)
class Query:
    text: str
    category: str
    relevant: frozenset[str] = field(default_factory=frozenset)


def receipt(
    merchant: str, lines: list[tuple[str, str]], total: str, footer: str
) -> Callable[[], Image.Image]:
    def render() -> Image.Image:
        im = Image.new("RGB", (720, 1000), (236, 236, 236))
        d = ImageDraw.Draw(im)
        d.rectangle((140, 40, 580, 960), fill="white")
        title, body = font(SANS_BOLD, 26), font(MONO, 20)
        d.text((360, 90), merchant, fill="black", font=title, anchor="mm")
        y = 160
        for item, price in lines:
            d.text((170, y), item, fill="black", font=body)
            d.text((550, y), price, fill="black", font=body, anchor="ra")
            y += 40
        d.line((170, y + 10, 550, y + 10), fill="black", width=2)
        d.text((170, y + 30), "TOTAL", fill="black", font=font(MONO, 24))
        d.text((550, y + 30), total, fill="black", font=font(MONO, 24), anchor="ra")
        d.text((360, y + 110), footer, fill=(80, 80, 80), font=body, anchor="mm")
        return im

    return render


def chat(title: str, messages: list[tuple[bool, str]]) -> Callable[[], Image.Image]:
    def render() -> Image.Image:
        im = Image.new("RGB", (720, 1280), (234, 229, 222))
        d = ImageDraw.Draw(im)
        d.rectangle((0, 0, 720, 110), fill=(7, 94, 84))
        d.text((40, 55), title, fill="white", font=font(SANS_BOLD, 30), anchor="lm")
        body = font(SANS, 24)
        y = 150
        for mine, text in messages:
            wrapped = textwrap.wrap(text, 30)
            h = 24 + 34 * len(wrapped)
            w = 40 + max(d.textlength(line, font=body) for line in wrapped)
            x0 = 680 - w if mine else 40
            colour = (220, 248, 198) if mine else "white"
            d.rounded_rectangle((x0, y, x0 + w, y + h), radius=18, fill=colour)
            for i, line in enumerate(wrapped):
                d.text((x0 + 20, y + 14 + 34 * i), line, fill="black", font=body)
            y += h + 26
        return im

    return render


def editor(filename: str, code: str, dark: bool = True) -> Callable[[], Image.Image]:
    def render() -> Image.Image:
        bg, fg, gutter = (
            ((30, 30, 30), (212, 212, 212), (110, 110, 110))
            if dark
            else ("white", (30, 30, 30), (160, 160, 160))
        )
        im = Image.new("RGB", (1440, 900), bg)
        d = ImageDraw.Draw(im)
        d.rectangle((0, 0, 1440, 44), fill=(45, 45, 45) if dark else (230, 230, 230))
        d.text((24, 22), filename, fill=fg, font=font(SANS, 20), anchor="lm")
        mono = font(MONO, 22)
        keywords = (
            "def ",
            "func ",
            "return",
            "import",
            "package",
            "if ",
            "for ",
            "const ",
            "class ",
        )
        for i, line in enumerate(code.strip("\n").split("\n")):
            y = 70 + i * 34
            d.text((20, y), f"{i + 1:>3}", fill=gutter, font=mono)
            colour = (86, 156, 214) if line.strip().startswith(keywords) else fg
            d.text((90, y), line, fill=colour, font=mono)
        return im

    return render


def terminal(lines: list[str]) -> Callable[[], Image.Image]:
    def render() -> Image.Image:
        im = Image.new("RGB", (1280, 760), (12, 12, 12))
        d = ImageDraw.Draw(im)
        d.rectangle((0, 0, 1280, 36), fill=(48, 48, 48))
        for i, colour in enumerate(((255, 95, 86), (255, 189, 46), (39, 201, 63))):
            d.ellipse((16 + i * 28, 10, 32 + i * 28, 26), fill=colour)
        mono = font(MONO, 22)
        for i, line in enumerate(lines):
            colour = (80, 250, 123) if line.startswith("$") else (230, 230, 230)
            d.text((24, 60 + i * 34), line, fill=colour, font=mono)
        return im

    return render


def dialog(title: str, message: str, button: str) -> Callable[[], Image.Image]:
    def render() -> Image.Image:
        im = Image.new("RGB", (1280, 800), (60, 63, 65))
        d = ImageDraw.Draw(im)
        d.rounded_rectangle((340, 230, 940, 570), radius=14, fill=(245, 245, 245))
        d.ellipse((380, 280, 440, 340), fill=(220, 53, 69))
        d.text((410, 310), "!", fill="white", font=font(SANS_BOLD, 40), anchor="mm")
        d.text((470, 295), title, fill="black", font=font(SANS_BOLD, 28))
        for i, line in enumerate(textwrap.wrap(message, 34)):
            d.text((470, 350 + i * 34), line, fill=(50, 50, 50), font=font(SANS, 22))
        d.rounded_rectangle((780, 500, 910, 548), radius=8, fill=(13, 110, 253))
        d.text((845, 524), button, fill="white", font=font(SANS_BOLD, 20), anchor="mm")
        return im

    return render


def sms(sender: str, text: str) -> Callable[[], Image.Image]:
    def render() -> Image.Image:
        im = Image.new("RGB", (720, 1280), "white")
        d = ImageDraw.Draw(im)
        d.rectangle((0, 0, 720, 120), fill=(245, 245, 245))
        d.text((360, 70), sender, fill="black", font=font(SANS_BOLD, 30), anchor="mm")
        body = font(SANS, 26)
        wrapped = textwrap.wrap(text, 28)
        h = 40 + 44 * len(wrapped)
        d.rounded_rectangle((40, 200, 600, 200 + h), radius=24, fill=(233, 233, 235))
        for i, line in enumerate(wrapped):
            d.text((70, 220 + 44 * i), line, fill="black", font=body)
        return im

    return render


def document(title: str, paragraphs: list[str]) -> Callable[[], Image.Image]:
    def render() -> Image.Image:
        im = Image.new("RGB", (1000, 1300), (220, 220, 220))
        d = ImageDraw.Draw(im)
        d.rectangle((90, 50, 910, 1260), fill="white")
        d.text((140, 110), title, fill="black", font=font(SANS_BOLD, 32))
        body = font(SANS, 22)
        y = 190
        for p in paragraphs:
            for line in textwrap.wrap(p, 62):
                d.text((140, y), line, fill=(30, 30, 30), font=body)
                y += 34
            y += 24
        return im

    return render


def bar_chart(title: str, labels: list[str], values: list[int]) -> Callable[[], Image.Image]:
    def render() -> Image.Image:
        im = Image.new("RGB", (1200, 800), "white")
        d = ImageDraw.Draw(im)
        d.text((600, 50), title, fill="black", font=font(SANS_BOLD, 32), anchor="mm")
        d.line((120, 700, 1120, 700), fill="black", width=3)
        d.line((120, 120, 120, 700), fill="black", width=3)
        step = 1000 / len(values)
        for i, (label, v) in enumerate(zip(labels, values, strict=True)):
            x = 120 + step * i + step * 0.2
            d.rectangle((x, 700 - v * 5, x + step * 0.6, 700), fill=(54, 162, 235))
            d.text((x + step * 0.3, 730), label, fill="black", font=font(SANS, 22), anchor="mm")
        return im

    return render


def line_chart(title: str, points: list[int]) -> Callable[[], Image.Image]:
    def render() -> Image.Image:
        im = Image.new("RGB", (1200, 800), "white")
        d = ImageDraw.Draw(im)
        d.text((600, 50), title, fill="black", font=font(SANS_BOLD, 32), anchor="mm")
        for y in range(160, 701, 90):
            d.line((120, y, 1120, y), fill=(225, 225, 225), width=1)
        d.line((120, 700, 1120, 700), fill="black", width=3)
        d.line((120, 120, 120, 700), fill="black", width=3)
        xs = [120 + 1000 * i / (len(points) - 1) for i in range(len(points))]
        coords = [(x, 700 - p * 5) for x, p in zip(xs, points, strict=True)]
        d.line(coords, fill=(255, 99, 132), width=5)
        for x, y in coords:
            d.ellipse((x - 7, y - 7, x + 7, y + 7), fill=(255, 99, 132))
        return im

    return render


def pie_chart(title: str, parts: list[tuple[str, int]]) -> Callable[[], Image.Image]:
    def render() -> Image.Image:
        im = Image.new("RGB", (1200, 800), "white")
        d = ImageDraw.Draw(im)
        d.text((600, 50), title, fill="black", font=font(SANS_BOLD, 32), anchor="mm")
        colours = [(255, 99, 132), (54, 162, 235), (255, 206, 86), (75, 192, 192), (153, 102, 255)]
        total, start = sum(v for _, v in parts), -90.0
        for i, (label, v) in enumerate(parts):
            end = start + 360 * v / total
            d.pieslice((300, 140, 860, 700), start, end, fill=colours[i % len(colours)])
            mid = math.radians((start + end) / 2)
            d.text(
                (580 + 330 * math.cos(mid), 420 + 330 * math.sin(mid)),
                label,
                fill="black",
                font=font(SANS, 22),
                anchor="mm",
            )
            start = end
        return im

    return render


def settings(title: str, rows: list[tuple[str, str, bool]]) -> Callable[[], Image.Image]:
    def render() -> Image.Image:
        im = Image.new("RGB", (720, 1280), (242, 242, 247))
        d = ImageDraw.Draw(im)
        d.text((40, 90), title, fill="black", font=font(SANS_BOLD, 40))
        for i, (label, detail, on) in enumerate(rows):
            y = 190 + i * 110
            d.rectangle((0, y, 720, y + 100), fill="white")
            d.text((40, y + 22), label, fill="black", font=font(SANS, 28))
            d.text((40, y + 62), detail, fill=(130, 130, 130), font=font(SANS, 20))
            d.rounded_rectangle(
                (590, y + 30, 680, y + 72), radius=21, fill=(52, 199, 89) if on else (200, 200, 200)
            )
            cx = 659 if on else 611
            d.ellipse((cx - 19, y + 32, cx + 19, y + 70), fill="white")
        return im

    return render


def note(title: str, lines: list[str]) -> Callable[[], Image.Image]:
    def render() -> Image.Image:
        im = Image.new("RGB", (900, 1100), (255, 251, 230))
        d = ImageDraw.Draw(im)
        d.text((60, 70), title, fill="black", font=font(SANS_BOLD, 34))
        y = 150
        for line in lines:
            for part in textwrap.wrap(line, 52):
                d.text((60, y), part, fill=(40, 40, 40), font=font(SANS, 24))
                y += 38
            y += 14
        return im

    return render


def boarding_pass(
    airline: str, route: str, fields: list[tuple[str, str]]
) -> Callable[[], Image.Image]:
    def render() -> Image.Image:
        im = Image.new("RGB", (1200, 560), (240, 240, 240))
        d = ImageDraw.Draw(im)
        d.rounded_rectangle((40, 40, 1160, 520), radius=24, fill="white")
        d.rounded_rectangle((40, 40, 1160, 140), radius=24, fill=(0, 44, 119))
        d.text((80, 90), airline, fill="white", font=font(SANS_BOLD, 34), anchor="lm")
        d.text((1120, 90), "BOARDING PASS", fill="white", font=font(SANS_BOLD, 26), anchor="rm")
        d.text((80, 200), route, fill="black", font=font(SANS_BOLD, 52))
        for i, (label, value) in enumerate(fields):
            x = 80 + (i % 4) * 270
            y = 320 + (i // 4) * 100
            d.text((x, y), label, fill=(120, 120, 120), font=font(SANS, 20))
            d.text((x, y + 30), value, fill="black", font=font(SANS_BOLD, 30))
        return im

    return render


def shots() -> list[Shot]:
    return [
        Shot(
            "receipt-cafe.png",
            "money",
            receipt(
                "Brew & Bean Cafe",
                [("Cappuccino x2", "360.00"), ("Butter Croissant", "85.00")],
                "445.00",
                "Paid via UPI. Thank you!",
            ),
        ),
        Shot(
            "receipt-coldbrew.png",
            "money",
            receipt(
                "Blue Tokai Coffee Roasters",
                [("Cold Brew Tonic", "260.00"), ("Banana Bread", "140.00")],
                "400.00",
                "Visit again",
            ),
        ),
        Shot(
            "receipt-grocery.png",
            "money",
            receipt(
                "FreshMart Supermarket",
                [
                    ("Milk 1L", "64.00"),
                    ("Whole Wheat Bread", "45.00"),
                    ("Eggs (12)", "96.00"),
                    ("GST 5%", "10.25"),
                ],
                "215.25",
                "Card ending 4417",
            ),
        ),
        Shot(
            "receipt-restaurant.png",
            "money",
            receipt(
                "Spice Route Restaurant",
                [
                    ("Paneer Tikka", "320.00"),
                    ("Butter Naan x3", "150.00"),
                    ("Service charge", "47.00"),
                ],
                "517.00",
                "Table 12 - Server: Ravi",
            ),
        ),
        Shot(
            "receipt-invoice.png",
            "money",
            receipt(
                "Amazon.in Tax Invoice",
                [
                    ("Invoice INV-2026-0042", ""),
                    ("Wireless Headphones", "2,499.00"),
                    ("Delivery", "0.00"),
                ],
                "2,499.00",
                "Order shipped via Delhivery",
            ),
        ),
        Shot(
            "receipt-cab.png",
            "money",
            receipt(
                "Uber Trip Receipt",
                [("Base fare", "120.00"), ("Distance 14.2 km", "198.00"), ("Time 32 min", "64.00")],
                "382.00",
                "Thanks for riding, Aditya",
            ),
        ),
        Shot(
            "boarding-indigo.png",
            "travel",
            boarding_pass(
                "IndiGo",
                "BLR  ->  DEL",
                [
                    ("FLIGHT", "6E 2134"),
                    ("SEAT", "14C"),
                    ("GATE", "22"),
                    ("BOARDING", "06:10"),
                    ("PASSENGER", "A BHALERAO"),
                    ("PNR", "QX7LMB"),
                ],
            ),
        ),
        Shot(
            "boarding-airindia.png",
            "travel",
            boarding_pass(
                "Air India",
                "BOM  ->  SFO",
                [
                    ("FLIGHT", "AI 179"),
                    ("SEAT", "32A"),
                    ("GATE", "C14"),
                    ("BOARDING", "01:35"),
                    ("PASSENGER", "A BHALERAO"),
                    ("PNR", "KD82PZ"),
                ],
            ),
        ),
        Shot(
            "chat-dinner.png",
            "message",
            chat(
                "Rohan",
                [
                    (False, "Are we still on for dinner at 8 tonight?"),
                    (True, "Yes! Booked a table at Toit for four"),
                    (False, "Perfect, see you there"),
                ],
            ),
        ),
        Shot(
            "chat-review.png",
            "message",
            chat(
                "Priya (work)",
                [
                    (False, "Can you review my pull request before standup?"),
                    (True, "Sure, looking at it now"),
                    (False, "Thanks, the migration part is tricky"),
                ],
            ),
        ),
        Shot(
            "chat-mom.png",
            "message",
            chat(
                "Mom",
                [
                    (False, "Call me when you reach home"),
                    (True, "Reached, will call in 10 minutes"),
                    (False, "Okay beta, eat something"),
                ],
            ),
        ),
        Shot(
            "chat-birthday.png",
            "message",
            chat(
                "Friends",
                [
                    (False, "Happy birthday Aditya!!"),
                    (False, "Party at my place on Saturday"),
                    (True, "Thank you all, I will bring the cake"),
                ],
            ),
        ),
        Shot(
            "code-python.png",
            "dev",
            editor(
                "users.py",
                """
import requests

def fetch_user(user_id):
    response = requests.get(f"/api/users/{user_id}")
    response.raise_for_status()
    return response.json()

class UserCache:
    def __init__(self):
        self.items = {}
""",
            ),
        ),
        Shot(
            "code-go.png",
            "dev",
            editor(
                "server.go",
                """
package main

import "net/http"

func main() {
    http.HandleFunc("/health", healthHandler)
    http.ListenAndServe(":8080", nil)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
}
""",
            ),
        ),
        Shot(
            "code-js-light.png",
            "dev",
            editor(
                "app.js",
                """
const express = require("express");
const app = express();

app.get("/orders/:id", async (req, res) => {
  const order = await db.orders.find(req.params.id);
  return res.json(order);
});

app.listen(3000);
""",
                dark=False,
            ),
        ),
        Shot(
            "terminal-git.png",
            "dev",
            terminal(
                [
                    "$ git push origin main",
                    "To github.com:arbhalerao/picseek.git",
                    " ! [rejected]        main -> main (fetch first)",
                    "error: failed to push some refs",
                    "hint: Updates were rejected because the remote contains work",
                    "$ git pull --rebase",
                ]
            ),
        ),
        Shot(
            "terminal-npm.png",
            "dev",
            terminal(
                [
                    "$ npm install",
                    "added 312 packages, and audited 313 packages in 9s",
                    "41 packages are looking for funding",
                    "found 0 vulnerabilities",
                    "$ npm run build",
                    "Compiled successfully in 4.2s",
                ]
            ),
        ),
        Shot(
            "error-timeout.png",
            None,
            dialog(
                "Connection timed out",
                "Unable to reach the server. Check your internet connection and try again.",
                "Retry",
            ),
        ),
        Shot(
            "error-traceback.png",
            "dev",
            terminal(
                [
                    "$ python app.py",
                    "Traceback (most recent call last):",
                    '  File "app.py", line 42, in main',
                    "    name = payload['user_id']",
                    "KeyError: 'user_id'",
                ]
            ),
        ),
        Shot(
            "error-404.png",
            None,
            dialog(
                "404 Page not found",
                "The page you are looking for does not exist or has been moved.",
                "Go home",
            ),
        ),
        Shot(
            "error-payment.png",
            "money",
            dialog(
                "Payment failed",
                "Your bank declined the transaction. No money was charged. Please try again.",
                "Try again",
            ),
        ),
        Shot(
            "otp-hdfc.png",
            "security",
            sms(
                "HDFCBK",
                "482913 is your OTP for login to HDFC NetBanking. Do not share it with anyone.",
            ),
        ),
        Shot(
            "otp-google.png",
            "security",
            sms("Google", "G-771204 is your Google verification code."),
        ),
        Shot(
            "otp-swiggy.png",
            "security",
            sms("SWIGGY", "Use 559031 to verify your Swiggy account. Valid for 10 minutes."),
        ),
        Shot(
            "doc-climate.png",
            "document",
            document(
                "Climate and Urban Heat",
                [
                    "Cities absorb and retain more heat than surrounding rural areas, a phenomenon known as the urban heat island effect.",
                    "Green roofs, reflective pavements and street trees can reduce peak summer temperatures by several degrees.",
                ],
            ),
        ),
        Shot(
            "doc-lease.png",
            "document",
            document(
                "Residential Lease Agreement",
                [
                    "This agreement is made between the landlord and the tenant for the property located at Flat 302, Koramangala.",
                    "The tenant shall pay a monthly rent of Rs. 28,000 on or before the fifth day of each month.",
                    "Either party may terminate this agreement with two months written notice.",
                ],
            ),
        ),
        Shot(
            "doc-abstract.png",
            "document",
            document(
                "Abstract",
                [
                    "We present a method for retrieving screenshots by their textual and visual content using optical character recognition and contrastive image embeddings.",
                    "Experiments show that hybrid ranking improves recall over lexical search alone.",
                ],
            ),
        ),
        Shot(
            "chart-revenue.png",
            "chart",
            bar_chart(
                "Monthly Revenue 2026",
                ["Jan", "Feb", "Mar", "Apr", "May", "Jun"],
                [40, 55, 62, 48, 80, 95],
            ),
        ),
        Shot(
            "chart-visitors.png",
            "chart",
            line_chart("Website Visitors", [20, 35, 30, 50, 65, 60, 85, 90]),
        ),
        Shot(
            "chart-expenses.png",
            "chart",
            pie_chart("Expenses", [("Rent", 40), ("Food", 25), ("Travel", 15), ("Other", 20)]),
        ),
        Shot(
            "settings-wifi.png",
            None,
            settings(
                "Wi-Fi",
                [
                    ("Wi-Fi", "On", True),
                    ("Home_5G", "Connected, secured", True),
                    ("Office-Guest", "Saved", False),
                    ("Airtel_Xstream", "Secured", False),
                ],
            ),
        ),
        Shot(
            "settings-bluetooth.png",
            None,
            settings(
                "Bluetooth",
                [
                    ("Bluetooth", "On", True),
                    ("Sony WH-1000XM5", "Connected", True),
                    ("Pixel Buds", "Not connected", False),
                ],
            ),
        ),
        Shot(
            "settings-battery.png",
            None,
            settings(
                "Battery",
                [
                    ("Battery Saver", "Off", False),
                    ("Battery percentage", "Show in status bar", True),
                    ("Adaptive Battery", "On", True),
                ],
            ),
        ),
        Shot(
            "split-payment.png",
            None,
            note(
                "Payment reminder",
                [
                    "Please con firm your pay ment before Friday",
                    "Tracking num ber will be shared by email",
                ],
            ),
        ),
        Shot(
            "split-deploy.png",
            None,
            note(
                "Release notes",
                [
                    "Deployment to stag ing finished at 14:20",
                    "Production rollout is scheduled for Monday",
                ],
            ),
        ),
        Shot(
            "note-recipe.png",
            "document",
            note(
                "Dal tadka",
                [
                    "Wash one cup of toor dal and pressure cook with turmeric.",
                    "Heat ghee, add cumin, garlic and dried red chillies.",
                    "Pour the tempering over the dal and garnish with coriander.",
                ],
            ),
        ),
        Shot(
            "note-football.png",
            "document",
            note(
                "Match report",
                [
                    "The home side equalised in the 89th minute after a long throw caused chaos in the box.",
                    "Their keeper made three saves in the second half.",
                ],
            ),
        ),
        Shot(
            "note-weather.png",
            "document",
            note(
                "Weekend forecast",
                [
                    "Light showers on Saturday morning, clearing by afternoon.",
                    "Sunday will be humid with highs of 31 degrees.",
                ],
            ),
        ),
        Shot(
            "note-books.png",
            "document",
            note(
                "Reading list",
                [
                    "Designing Data-Intensive Applications",
                    "The Pragmatic Programmer",
                    "Thinking, Fast and Slow",
                ],
            ),
        ),
        Shot(
            "note-gym.png",
            "document",
            note(
                "Workout plan",
                [
                    "Monday: squats 5x5, bench press 5x5",
                    "Wednesday: deadlifts 3x5, pull ups",
                    "Friday: overhead press, rows, planks",
                ],
            ),
        ),
        Shot(
            "note-meeting.png",
            "document",
            note(
                "Team sync",
                [
                    "Ship the search latency fix this sprint.",
                    "Ravi to update the onboarding docs.",
                    "Next sync on Thursday at 11.",
                ],
            ),
        ),
        Shot(
            "note-groceries.png",
            "document",
            note("To buy", ["Onions, tomatoes, ginger", "Basmati rice 5kg", "Dish soap"]),
        ),
        Shot(
            "note-travel.png",
            "document",
            note(
                "Goa itinerary",
                [
                    "Day 1: Baga beach and Fort Aguada",
                    "Day 2: Old Goa churches and spice plantation",
                    "Day 3: Palolem",
                ],
            ),
        ),
    ]


def q(text: str, category: str, *relevant: str) -> Query:
    return Query(text, category, frozenset(relevant))


def queries() -> list[Query]:
    return [
        q("cappuccino", "keyword", "receipt-cafe.png"),
        q("Toit", "keyword", "chat-dinner.png"),
        q("INV-2026-0042", "keyword", "receipt-invoice.png"),
        q("KeyError user_id", "keyword", "error-traceback.png"),
        q("6E 2134", "keyword", "boarding-indigo.png"),
        q("HDFC NetBanking", "keyword", "otp-hdfc.png"),
        q("Home_5G", "keyword", "settings-wifi.png"),
        q("git push rejected", "keyword", "terminal-git.png"),
        q("Koramangala rent", "keyword", "doc-lease.png"),
        q("cappu", "partial", "receipt-cafe.png"),
        q("NetBank", "partial", "otp-hdfc.png"),
        q("Delhiv", "partial", "receipt-invoice.png"),
        q("lnvoice", "misread", "receipt-invoice.png"),
        q("0TP HDFC", "misread", "otp-hdfc.png"),
        q("confirm payment", "misread", "split-payment.png"),
        q("staging", "misread", "split-deploy.png"),
        q("tracking number", "misread", "split-payment.png"),
        q("coffee shop bill", "meaning", "receipt-cafe.png", "receipt-coldbrew.png"),
        q("flight ticket", "meaning", "boarding-indigo.png", "boarding-airindia.png"),
        q(
            "verification code message",
            "meaning",
            "otp-hdfc.png",
            "otp-google.png",
            "otp-swiggy.png",
        ),
        q("program crashed with an exception", "meaning", "error-traceback.png"),
        q("dinner reservation with friends", "meaning", "chat-dinner.png"),
        q("wireless network settings", "meaning", "settings-wifi.png"),
        q("supermarket shopping", "meaning", "receipt-grocery.png"),
        q("taxi ride cost", "meaning", "receipt-cab.png"),
        q("house rental contract", "meaning", "doc-lease.png"),
        q("headphones bluetooth pairing", "meaning", "settings-bluetooth.png"),
        q("lentil curry recipe", "meaning", "note-recipe.png"),
        q("a bar chart", "visual", "chart-revenue.png"),
        q("a line graph", "visual", "chart-visitors.png"),
        q("a pie chart", "visual", "chart-expenses.png"),
        q("a dark mode code editor", "visual", "code-python.png", "code-go.png"),
        q(
            "chat message bubbles",
            "visual",
            "chat-dinner.png",
            "chat-review.png",
            "chat-mom.png",
            "chat-birthday.png",
        ),
        q(
            "a terminal window",
            "visual",
            "terminal-git.png",
            "terminal-npm.png",
            "error-traceback.png",
        ),
        q("an error dialog", "visual", "error-timeout.png", "error-404.png", "error-payment.png"),
        q("xqzv purple elephant tornado", "negative"),
        q("a horse grazing in a meadow", "negative"),
        q("quantum chromodynamics lecture", "negative"),
        q("snowboarding in the alps", "negative"),
    ]


def generate(directory: Path) -> list[Shot]:
    directory.mkdir(parents=True, exist_ok=True)
    random.seed(0)
    corpus = shots()
    for shot in corpus:
        shot.render().save(directory / shot.name)
    return corpus
