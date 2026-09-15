"""
Telegram entrypoint for the safety-incident bot.

Flow: worker message -> AI triage (always-on) -> if Medium+, log on-chain
-> alert the safety officer, with Critical delivered immediately/flagged.

Human-in-the-loop: the AI never decides the response, only classifies and
surfaces. The safety officer makes the actual call.

Access control: on-chain writes pay gas from a single relayer wallet, so
first-time senders must provide an access code before their messages
trigger AI triage or chain writes. Authorized users are remembered for
the life of the process (resets on restart).
"""

import logging
import os

from telegram import InlineKeyboardButton, InlineKeyboardMarkup, Update
from telegram.ext import Application, ContextTypes, MessageHandler, filters

from chain import log_incident_on_chain
from triage import Severity, classify_incident

logging.basicConfig(level=logging.INFO)
logging.getLogger("httpx").setLevel(logging.WARNING)
logging.getLogger("httpcore").setLevel(logging.WARNING)

logger = logging.getLogger(__name__)

BOT_TOKEN = os.environ["TELEGRAM_BOT_TOKEN"]
SAFETY_OFFICER_CHAT_ID = os.environ["SAFETY_OFFICER_CHAT_ID"]

ACCESS_CODE = os.environ.get("BOT_ACCESS_CODE", "")
_raw_allowed = os.environ.get("ALLOWED_USER_IDS", "")
authorized_users = {int(uid.strip()) for uid in _raw_allowed.split(",") if uid.strip()}


async def handle_report(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    user_id = update.effective_user.id
    text = update.message.text

    if user_id not in authorized_users:
        if ACCESS_CODE and text.strip() == ACCESS_CODE:
            authorized_users.add(user_id)
            logger.info("Authorized new user_id=%s via access code", user_id)
            await update.message.reply_text(
                "Access granted. Send your next message as an incident report."
            )
        else:
            logger.info("Blocked message from unauthorized user_id=%s", user_id)
            await update.message.reply_text(
                "This demo requires an access code. If you have one, send it now."
            )
        return

    report_text = text
    worker_chat_id = update.effective_chat.id

    await update.message.reply_text("Got it, logging and assessing now.")

    try:
        result = classify_incident(report_text)
    except Exception:
        logger.exception("Triage classification failed")
        await update.message.reply_text(
            "Couldn't complete the assessment right now, but your report was received. "
            "Please also notify your supervisor directly if this is urgent."
        )
        return

    chain_info = None
    officer_notified = False

    if result.should_log_on_chain():
        try:
            chain_info = log_incident_on_chain(result, update.effective_user.id, network="botchain_mainnet")
        except Exception:
            logger.exception("On-chain logging failed for a %s incident", result.severity.name)

        await notify_safety_officer(context, result, chain_info)
        officer_notified = True

    if officer_notified:
        reply = (
            f"Recorded as {result.severity.name}. Thanks for reporting, "
            f"a safety officer has been notified."
        )
    else:
        reply = f"Recorded as {result.severity.name}. Thanks for reporting."

    await update.message.reply_text(reply)


async def notify_safety_officer(context: ContextTypes.DEFAULT_TYPE, result, chain_info) -> None:
    severity_emoji = {
        "CRITICAL": "🚨",
        "HIGH": "⚠️",
        "MEDIUM": "⚡",
    }.get(result.severity.name, "ℹ️")

    text = (
        f"{severity_emoji} <b>INCIDENT REPORTED: {result.severity.name}</b>\n\n"
        f"<b>Summary:</b> {result.summary}\n\n"
        f"<b>Driving Factor:</b> <code>{result.driving_factor}</code>\n\n"
        f"<b>AI Reasoning:</b>\n<i>{result.justification}</i>"
    )

    reply_markup = None

    if chain_info:
        keyboard = [[InlineKeyboardButton("🔗 View On-Chain Record", url=chain_info["explorer_url"])]]
        reply_markup = InlineKeyboardMarkup(keyboard)
    elif result.should_log_on_chain():
        text += "\n\n⚠️ <b>On-chain logging failed</b> — needs manual follow-up."

    await context.bot.send_message(
        chat_id=SAFETY_OFFICER_CHAT_ID,
        text=text,
        parse_mode="HTML",
        reply_markup=reply_markup,
    )


def main() -> None:
    app = Application.builder().token(BOT_TOKEN).build()
    app.add_handler(MessageHandler(filters.TEXT & ~filters.COMMAND, handle_report))
    logger.info("Safety-incident bot starting...")
    app.run_polling()


if __name__ == "__main__":
    main()