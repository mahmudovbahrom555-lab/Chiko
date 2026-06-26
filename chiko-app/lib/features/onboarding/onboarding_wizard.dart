import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:go_router/go_router.dart';
import 'package:file_picker/file_picker.dart';
import 'package:qr_flutter/qr_flutter.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:supabase_flutter/supabase_flutter.dart';
import 'package:url_launcher/url_launcher.dart';

// ── Constants ──────────────────────────────────────────────────────────────────

const _kStepCount = 4;

// ── Wizard shell ───────────────────────────────────────────────────────────────

class OnboardingWizard extends StatefulWidget {
  const OnboardingWizard({super.key});

  @override
  State<OnboardingWizard> createState() => _OnboardingWizardState();
}

class _OnboardingWizardState extends State<OnboardingWizard> {
  final _controller = PageController();
  int _current = 0;

  void _next() {
    if (_current < _kStepCount - 1) {
      _controller.nextPage(
        duration: const Duration(milliseconds: 300),
        curve: Curves.easeInOut,
      );
    } else {
      _finish();
    }
  }

  void _skip() => _next();

  Future<void> _finish() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setBool('onboarding_done', true);
    if (mounted) context.go('/chats');
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        child: Column(
          children: [
            // Progress indicator.
            LinearProgressIndicator(value: (_current + 1) / _kStepCount),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
              child: Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  Text('Шаг ${_current + 1} из $_kStepCount',
                      style: Theme.of(context).textTheme.bodySmall),
                  TextButton(
                    onPressed: _skip,
                    child: const Text('Пропустить'),
                  ),
                ],
              ),
            ),
            Expanded(
              child: PageView(
                controller: _controller,
                physics: const NeverScrollableScrollPhysics(),
                onPageChanged: (i) => setState(() => _current = i),
                children: [
                  _Step1Currency(onNext: _next),
                  _Step2Catalog(onNext: _next, onSkip: _skip),
                  _Step3Client(onNext: _next, onSkip: _skip),
                  _Step4GuestLink(onDone: _finish),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ── Step 1: Currency ───────────────────────────────────────────────────────────

class _Step1Currency extends StatefulWidget {
  final VoidCallback onNext;
  const _Step1Currency({required this.onNext});

  @override
  State<_Step1Currency> createState() => _Step1CurrencyState();
}

class _Step1CurrencyState extends State<_Step1Currency> {
  String _selected = 'UZS';
  bool _loading = false;

  final _currencies = ['UZS', 'USD', 'EUR', 'RUB', 'KZT'];

  Future<void> _save() async {
    setState(() => _loading = true);
    try {
      final userId = Supabase.instance.client.auth.currentUser?.id;
      if (userId != null) {
        await Supabase.instance.client
            .from('producers')
            .update({'catalog_currency': _selected}).eq('id', userId);
      }
      widget.onNext();
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.all(24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text('Валюта каталога',
              style: Theme.of(context).textTheme.headlineSmall),
          const SizedBox(height: 8),
          const Text('Выберите валюту для отображения цен'),
          const SizedBox(height: 8),
          const Text('Валюту можно изменить в настройках в любое время',
              style: TextStyle(color: Colors.grey)),
          const SizedBox(height: 32),
          ..._currencies.map((c) => RadioListTile<String>(
                title: Text(c),
                value: c,
                groupValue: _selected,
                onChanged: (v) => setState(() => _selected = v!),
              )),
          const Spacer(),
          SizedBox(
            width: double.infinity,
            child: ElevatedButton(
              onPressed: _loading ? null : _save,
              child: _loading
                  ? const CircularProgressIndicator(strokeWidth: 2)
                  : const Text('Продолжить'),
            ),
          ),
        ],
      ),
    );
  }
}

// ── Step 2: Catalog import ─────────────────────────────────────────────────────

class _Step2Catalog extends StatefulWidget {
  final VoidCallback onNext;
  final VoidCallback onSkip;
  const _Step2Catalog({required this.onNext, required this.onSkip});

  @override
  State<_Step2Catalog> createState() => _Step2CatalogState();
}

class _Step2CatalogState extends State<_Step2Catalog> {
  bool _uploading = false;
  String? _result;

  Future<void> _downloadTemplate() async {
    // GET /api/catalog/template — already implemented in Step 2.2.
    final session = Supabase.instance.client.auth.currentSession;
    if (session == null) return;
    final uri = Uri.parse('https://api.chiko.uz/api/catalog/template');
    if (await canLaunchUrl(uri)) await launchUrl(uri);
  }

  Future<void> _uploadFile() async {
    final result = await FilePicker.platform.pickFiles(
      type: FileType.custom,
      allowedExtensions: ['xlsx', 'xls'],
    );
    if (result == null || result.files.isEmpty) return;

    setState(() => _uploading = true);
    try {
      final bytes = result.files.first.bytes;
      if (bytes == null) return;

      final session = Supabase.instance.client.auth.currentSession;
      if (session == null) return;

      // POST /api/catalog/import — multipart form.
      final response = await Supabase.instance.client.functions.invoke(
        'catalog-import',
        body: bytes,
        headers: {
          'Content-Type': 'application/octet-stream',
          'Authorization': 'Bearer ${session.accessToken}',
        },
      );
      setState(() => _result = 'Импортировано успешно: ${response.data}');
    } catch (e) {
      setState(() => _result = 'Ошибка: $e');
    } finally {
      if (mounted) setState(() => _uploading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.all(24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text('Загрузите каталог',
              style: Theme.of(context).textTheme.headlineSmall),
          const SizedBox(height: 8),
          const Text('Импортируйте товары из Excel-файла'),
          const SizedBox(height: 32),
          OutlinedButton.icon(
            onPressed: _downloadTemplate,
            icon: const Icon(Icons.download),
            label: const Text('Скачать шаблон'),
          ),
          const SizedBox(height: 16),
          ElevatedButton.icon(
            onPressed: _uploading ? null : _uploadFile,
            icon: _uploading
                ? const SizedBox(
                    width: 16, height: 16,
                    child: CircularProgressIndicator(strokeWidth: 2))
                : const Icon(Icons.upload_file),
            label: const Text('Загрузить файл'),
          ),
          if (_result != null) ...[
            const SizedBox(height: 16),
            Text(_result!,
                style: TextStyle(
                    color: _result!.startsWith('Ошибка')
                        ? Colors.red
                        : Colors.green)),
          ],
          const Spacer(),
          Row(children: [
            Expanded(
              child: OutlinedButton(
                onPressed: widget.onSkip,
                child: const Text('Пропустить'),
              ),
            ),
            const SizedBox(width: 16),
            Expanded(
              child: ElevatedButton(
                onPressed: widget.onNext,
                child: const Text('Продолжить'),
              ),
            ),
          ]),
        ],
      ),
    );
  }
}

// ── Step 3: Add first client ───────────────────────────────────────────────────

class _Step3Client extends StatefulWidget {
  final VoidCallback onNext;
  final VoidCallback onSkip;
  const _Step3Client({required this.onNext, required this.onSkip});

  @override
  State<_Step3Client> createState() => _Step3ClientState();
}

class _Step3ClientState extends State<_Step3Client> {
  final _phoneCtrl = TextEditingController();
  bool _loading = false;
  String? _error;

  Future<void> _addClient() async {
    final phone = _phoneCtrl.text.trim();
    if (phone.isEmpty) {
      setState(() => _error = 'Введите номер телефона');
      return;
    }
    setState(() { _loading = true; _error = null; });
    try {
      await Supabase.instance.client.functions.invoke('create-chat',
          body: {'phone': phone});
      widget.onNext();
    } catch (e) {
      setState(() => _error = e.toString());
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.all(24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text('Добавьте первого клиента',
              style: Theme.of(context).textTheme.headlineSmall),
          const SizedBox(height: 8),
          const Text('Введите номер телефона клиента'),
          const SizedBox(height: 32),
          TextField(
            controller: _phoneCtrl,
            keyboardType: TextInputType.phone,
            decoration: const InputDecoration(
              labelText: 'Номер телефона',
              hintText: '+998 90 123 45 67',
            ),
          ),
          if (_error != null) ...[
            const SizedBox(height: 8),
            Text(_error!, style: const TextStyle(color: Colors.red)),
          ],
          const Spacer(),
          Row(children: [
            Expanded(
              child: OutlinedButton(
                onPressed: widget.onSkip,
                child: const Text('Пропустить'),
              ),
            ),
            const SizedBox(width: 16),
            Expanded(
              child: ElevatedButton(
                onPressed: _loading ? null : _addClient,
                child: _loading
                    ? const CircularProgressIndicator(strokeWidth: 2)
                    : const Text('Добавить'),
              ),
            ),
          ]),
        ],
      ),
    );
  }
}

// ── Step 4: Guest link / QR ────────────────────────────────────────────────────

class _Step4GuestLink extends StatefulWidget {
  final VoidCallback onDone;
  const _Step4GuestLink({required this.onDone});

  @override
  State<_Step4GuestLink> createState() => _Step4GuestLinkState();
}

class _Step4GuestLinkState extends State<_Step4GuestLink> {
  String? _deepLink;
  String? _webLink;
  bool _loading = true;
  bool _showQR = false;

  @override
  void initState() {
    super.initState();
    _loadGuestLink();
  }

  Future<void> _loadGuestLink() async {
    try {
      final resp = await Supabase.instance.client.functions.invoke(
        'guest-link', // maps to GET /api/producers/me/guest-link
      );
      final data = resp.data as Map<String, dynamic>?;
      if (data != null) {
        setState(() {
          _deepLink = data['deep_link'] as String?;
          _webLink  = data['web_link'] as String?;
        });
      }
    } catch (_) {
      // Non-fatal — user can share later from Settings.
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _copyLink() async {
    if (_webLink == null) return;
    await Clipboard.setData(ClipboardData(text: _webLink!));
    if (mounted) {
      ScaffoldMessenger.of(context)
          .showSnackBar(const SnackBar(content: Text('Ссылка скопирована')));
    }
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.all(24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text('Гостевой каталог',
              style: Theme.of(context).textTheme.headlineSmall),
          const SizedBox(height: 8),
          const Text('Поделитесь ссылкой или QR-кодом с клиентами'),
          const SizedBox(height: 32),
          if (_loading)
            const Center(child: CircularProgressIndicator())
          else if (_showQR && _webLink != null) ...[
            Center(
              child: QrImageView(
                data: _webLink!,
                size: 200,
              ),
            ),
            const SizedBox(height: 16),
            Center(
              child: TextButton(
                onPressed: () => setState(() => _showQR = false),
                child: const Text('Скрыть QR'),
              ),
            ),
          ] else ...[
            if (_webLink != null)
              Container(
                padding: const EdgeInsets.all(12),
                decoration: BoxDecoration(
                  border: Border.all(color: Colors.grey.shade300),
                  borderRadius: BorderRadius.circular(8),
                ),
                child: Text(_webLink!, style: const TextStyle(fontSize: 12)),
              ),
            const SizedBox(height: 16),
            Row(children: [
              Expanded(
                child: OutlinedButton.icon(
                  onPressed: _copyLink,
                  icon: const Icon(Icons.copy),
                  label: const Text('Скопировать'),
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: OutlinedButton.icon(
                  onPressed: () => setState(() => _showQR = true),
                  icon: const Icon(Icons.qr_code),
                  label: const Text('QR-код'),
                ),
              ),
            ]),
          ],
          const Spacer(),
          SizedBox(
            width: double.infinity,
            child: ElevatedButton(
              onPressed: widget.onDone,
              child: const Text('Готово'),
            ),
          ),
        ],
      ),
    );
  }
}
