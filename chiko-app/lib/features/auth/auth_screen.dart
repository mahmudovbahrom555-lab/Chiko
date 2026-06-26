import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:supabase_flutter/supabase_flutter.dart';

class AuthScreen extends StatefulWidget {
  const AuthScreen({super.key});

  @override
  State<AuthScreen> createState() => _AuthScreenState();
}

class _AuthScreenState extends State<AuthScreen> {
  final _phoneCtrl = TextEditingController();
  final _otpCtrl   = TextEditingController();
  bool _otpSent    = false;
  bool _loading    = false;
  String? _error;

  Future<void> _sendOtp() async {
    setState(() { _loading = true; _error = null; });
    try {
      await Supabase.instance.client.auth.signInWithOtp(phone: _phoneCtrl.text.trim());
      setState(() { _otpSent = true; });
    } catch (e) {
      setState(() { _error = e.toString(); });
    } finally {
      setState(() { _loading = false; });
    }
  }

  Future<void> _verifyOtp() async {
    setState(() { _loading = true; _error = null; });
    try {
      await Supabase.instance.client.auth.verifyOTP(
        phone: _phoneCtrl.text.trim(),
        token: _otpCtrl.text.trim(),
        type: OtpType.sms,
      );
      // Bootstrap on server — fire-and-forget; failure is non-fatal.
      await _bootstrap();
      if (mounted) context.go('/onboarding');
    } catch (e) {
      setState(() { _error = e.toString(); });
    } finally {
      if (mounted) setState(() { _loading = false; });
    }
  }

  Future<void> _bootstrap() async {
    try {
      final session = Supabase.instance.client.auth.currentSession;
      if (session == null) return;
      // POST /api/auth/bootstrap on the Go backend — creates producer record + links pending chats.
      final client = HttpClient();
      final req = await client.postUrl(
          Uri.parse(const String.fromEnvironment('API_URL', defaultValue: 'https://api.chiko.uz') +
              '/api/auth/bootstrap'));
      req.headers.set(HttpHeaders.authorizationHeader, 'Bearer ${session.accessToken}');
      req.headers.set(HttpHeaders.contentTypeHeader, 'application/json');
      req.write('{}');
      await req.close();
      client.close();
    } catch (_) {
      // Non-fatal — producer record will be created on next authenticated request.
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Войти в Chiko')),
      body: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            TextField(
              controller: _phoneCtrl,
              keyboardType: TextInputType.phone,
              decoration: const InputDecoration(
                labelText: 'Номер телефона',
                hintText: '+998 90 123 45 67',
              ),
              enabled: !_otpSent,
            ),
            if (_otpSent) ...[
              const SizedBox(height: 16),
              TextField(
                controller: _otpCtrl,
                keyboardType: TextInputType.number,
                decoration: const InputDecoration(
                  labelText: 'Код из SMS',
                  hintText: 'XXXXXX',
                ),
              ),
            ],
            if (_error != null) ...[
              const SizedBox(height: 8),
              Text(_error!, style: const TextStyle(color: Colors.red)),
            ],
            const SizedBox(height: 24),
            SizedBox(
              width: double.infinity,
              child: ElevatedButton(
                onPressed: _loading ? null : (_otpSent ? _verifyOtp : _sendOtp),
                child: _loading
                    ? const CircularProgressIndicator(strokeWidth: 2)
                    : Text(_otpSent ? 'Войти' : 'Получить код'),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
