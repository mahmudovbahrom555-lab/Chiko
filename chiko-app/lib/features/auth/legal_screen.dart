import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:shared_preferences/shared_preferences.dart';

class LegalScreen extends StatefulWidget {
  const LegalScreen({super.key});

  @override
  State<LegalScreen> createState() => _LegalScreenState();
}

class _LegalScreenState extends State<LegalScreen> {
  bool _agreed = false;

  Future<void> _accept() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setBool('legal_accepted', true);
    if (mounted) context.go('/auth');
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Условия использования')),
      body: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Expanded(
              child: SingleChildScrollView(
                child: Text(
                  'Используя Chiko, вы соглашаетесь с тем, что:\n\n'
                  '1. Ваши данные обрабатываются для обеспечения работы сервиса.\n'
                  '2. Сообщения и данные о транзакциях хранятся на серверах.\n'
                  '3. Для уведомлений используются push-токены устройств.\n'
                  '4. Вы несёте ответственность за корректность вводимых данных.\n\n'
                  'Полный текст: https://chiko.uz/legal',
                ),
              ),
            ),
            CheckboxListTile(
              value: _agreed,
              onChanged: (v) => setState(() => _agreed = v ?? false),
              title: const Text(
                'Я согласен с условиями использования и политикой конфиденциальности',
              ),
              controlAffinity: ListTileControlAffinity.leading,
              contentPadding: EdgeInsets.zero,
            ),
            const SizedBox(height: 16),
            SizedBox(
              width: double.infinity,
              child: ElevatedButton(
                onPressed: _agreed ? _accept : null,
                child: const Text('Принять и продолжить'),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
