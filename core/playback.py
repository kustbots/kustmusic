import asyncio
import logging
import os
import time

from pyrogram.enums import ParseMode
from pyrogram.errors import UserAlreadyParticipant
from pyrogram.types import InlineKeyboardButton, InlineKeyboardMarkup
from pytgcalls import filters as fl
from pytgcalls.types import StreamEnded

import state
from clients import call_py, user_app
from core.api import download_song
from core.helpers import get_progress_bar, one_line_title, parse_duration_str

logger = logging.getLogger(__name__)

_ASSISTANT_JOIN_ERRORS = (
    "peeridinvalid", "peer_id_invalid", "channelprivate",
    "not in chat", "usernotparticipant", "groupcallinvalid",
    "invalid peer", "chatadminrequired", "channelinvalid",
    "channel_invalid",
)


async def _try_join_assistant(client, chat_id, status_msg):
    try:
        if status_msg:
            await status_msg.edit_text("🔄 <b>Joining assistant...</b>", parse_mode=ParseMode.HTML)
        try:
            invite_link = await client.export_chat_invite_link(chat_id)
        except Exception:
            link_obj = await client.create_chat_invite_link(chat_id)
            invite_link = link_obj.invite_link
        try:
            await user_app.join_chat(invite_link)
        except UserAlreadyParticipant:
            pass
        except Exception as join_err:
            err_s = str(join_err).lower()
            if "expired" in err_s or "invalid" in err_s or "hash" in err_s:
                new_link = await client.create_chat_invite_link(chat_id)
                await user_app.join_chat(new_link.invite_link)
            else:
                raise join_err
        await asyncio.sleep(2)
        return True
    except Exception as e:
        logger.warning(f"Assistant join failed for {chat_id}: {e}")
        return False


def _build_control_keyboard(chat_id, progress_bar):
    is_paused = chat_id in state.paused_chats
    toggle_btn = (
        InlineKeyboardButton(text="▶️ Resume", callback_data="resume")
        if is_paused else
        InlineKeyboardButton(text="⏸ Pause", callback_data="pause")
    )
    return InlineKeyboardMarkup([
        [InlineKeyboardButton(text=progress_bar, callback_data="progress")],
        [toggle_btn],
        [
            InlineKeyboardButton(text="⏭ Skip", callback_data="skip"),
            InlineKeyboardButton(text="⏹ Stop", callback_data="stop"),
        ],
    ])


async def update_progress_caption(chat_id, message, start_time, total_duration, base_caption):
    try:
        while True:
            elapsed = time.time() - start_time
            if total_duration > 0 and elapsed > total_duration:
                elapsed = total_duration

            progress_bar = get_progress_bar(elapsed, total_duration)
            new_keyboard = _build_control_keyboard(chat_id, progress_bar)

            try:
                await message.edit_caption(
                    caption=base_caption, reply_markup=new_keyboard, parse_mode=ParseMode.HTML
                )
            except Exception as e:
                if "MESSAGE_NOT_MODIFIED" not in str(e):
                    break

            if total_duration > 0 and elapsed >= total_duration:
                break
            await asyncio.sleep(10)
    except asyncio.CancelledError:
        pass
    except Exception as e:
        logger.error(f"Progress update error: {e}")


async def play_music_core(client, chat_id, song_info, status_msg=None, retry_attempt=False):
    try:
        file_path = song_info.get("file_path")
        if not file_path or not os.path.exists(file_path):
            if status_msg:
                await status_msg.edit_text("⬇️ <b>Downloading audio...</b>", parse_mode=ParseMode.HTML)
            file_path = await download_song(song_info["url"])
            if not file_path or not os.path.exists(file_path):
                if status_msg:
                    await status_msg.edit_text("❌ <b>Download failed.</b>", parse_mode=ParseMode.HTML)
                if chat_id in state.chat_queues and state.chat_queues[chat_id]:
                    state.chat_queues[chat_id].pop(0)
                    if state.chat_queues[chat_id]:
                        asyncio.create_task(
                            play_music_core(client, chat_id, state.chat_queues[chat_id][0], status_msg)
                        )
                return
            song_info["file_path"] = file_path

        if status_msg:
            await status_msg.edit_text("🎧 <b>Starting playback...</b>", parse_mode=ParseMode.HTML)

        try:
            await call_py.play(chat_id, file_path)
        except Exception as e:
            err_s = str(e).lower()
            is_chat_error = any(k in err_s for k in _ASSISTANT_JOIN_ERRORS)

            if is_chat_error and not retry_attempt:
                joined = await _try_join_assistant(client, chat_id, status_msg)
                if joined:
                    return await play_music_core(client, chat_id, song_info, status_msg, retry_attempt=True)

            ast_mention = f"@{state.ASSISTANT_USERNAME}" if state.ASSISTANT_USERNAME else "the assistant"
            verify_btn = InlineKeyboardMarkup([[
                InlineKeyboardButton("✅ I added the assistant — retry", callback_data="verify_assistant")
            ]])
            if status_msg:
                await status_msg.edit_text(
                    f"❌ <b>Playback failed:</b>\n<code>{e}</code>\n\n"
                    f"Add {ast_mention} to this group and the voice chat, then click below.",
                    parse_mode=ParseMode.HTML,
                    reply_markup=verify_btn,
                )
            return

        if chat_id in state.progress_tasks:
            state.progress_tasks[chat_id].cancel()
            del state.progress_tasks[chat_id]
        state.paused_chats.discard(chat_id)

        bot_name = client.me.first_name or "Music Bot"
        title_short = one_line_title(song_info["title"])
        total_duration = parse_duration_str(song_info.get("duration", "0"))

        base_caption = (
            f"<blockquote><b>🎧 {bot_name} · ᴍᴜsɪᴄ sᴛʀєᴀᴍɪɴɢ</b></blockquote>\n\n"
            f"<blockquote>🎵 <b>ᴛɪᴛʟᴇ:</b> {title_short}\n"
            f"👤 <b>ʀᴇǫᴜᴇsᴛᴇᴅ ʙʏ:</b> {song_info['req']}</blockquote>"
        )

        control_buttons = _build_control_keyboard(chat_id, get_progress_bar(0, total_duration))

        if status_msg:
            await status_msg.delete()

        player_message = None
        if song_info.get("thumb") and song_info["thumb"].startswith("http"):
            try:
                player_message = await client.send_photo(
                    chat_id,
                    photo=song_info["thumb"],
                    caption=base_caption,
                    reply_markup=control_buttons,
                    parse_mode=ParseMode.HTML,
                )
            except Exception:
                pass

        if not player_message:
            player_message = await client.send_message(
                chat_id,
                base_caption,
                reply_markup=control_buttons,
                parse_mode=ParseMode.HTML,
                disable_web_page_preview=True,
            )

        if player_message:
            task = asyncio.create_task(
                update_progress_caption(chat_id, player_message, time.time(), total_duration, base_caption)
            )
            state.progress_tasks[chat_id] = task

    except Exception as e:
        logger.error(f"Playback error in chat {chat_id}: {e}")
        if status_msg:
            try:
                await status_msg.edit_text(f"❌ <b>Error:</b> {e}", parse_mode=ParseMode.HTML)
            except Exception:
                pass
        if chat_id in state.chat_queues and state.chat_queues[chat_id]:
            state.chat_queues[chat_id].pop(0)


@call_py.on_update(fl.stream_end())
async def on_stream_end(_, update: StreamEnded):
    chat_id = update.chat_id

    if chat_id in state.progress_tasks:
        state.progress_tasks[chat_id].cancel()
        del state.progress_tasks[chat_id]

    if chat_id not in state.chat_queues or not state.chat_queues[chat_id]:
        try:
            await call_py.leave_call(chat_id)
        except Exception:
            pass
        return

    finished_song = state.chat_queues[chat_id].pop(0)
    fp = finished_song.get("file_path")
    if fp and os.path.exists(fp):
        try:
            os.remove(fp)
        except Exception:
            pass

    if state.chat_queues[chat_id]:
        next_song = state.chat_queues[chat_id][0]
        target_client = state.active_clients.get(next_song.get("bot_id"))
        if target_client:
            await play_music_core(target_client, chat_id, next_song)
        else:
            try:
                await call_py.leave_call(chat_id)
            except Exception:
                pass
    else:
        try:
            await call_py.leave_call(chat_id)
        except Exception:
            pass
